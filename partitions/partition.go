package partitions

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/danthegoodman1/FireScroll/internal"
	"github.com/danthegoodman1/FireScroll/utils"
	"github.com/dgraph-io/badger/v4"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

var (
	//go:embed create_kv.sql
	CreateKVTableStmt string

	//go:embed create_offset_keeper.sql
	CreateOffsetKeeperTableStmt string

	LastOffsetKey = []byte("last_offset")

	ErrInvalidKey = errors.New("invalid key")
)

const (

	// KeyPrefix is used to prefix every key in the local DB so that all internal values are protected.
	// ~ is a high unicode character so normal ASCII values will be below, that means a prefix scan of nothing
	// will always be in the user space.
	KeyPrefix = "~"
)

type (
	Partition struct {
		ID int32
		DB *badger.DB
		// TODO: Maybe atomic?
		LastOffset   *int64
		Namespace    string
		GCTicker     *time.Ticker
		BackupTicker *time.Ticker
		closeChan    chan struct{}
		BackupWg     sync.WaitGroup
		s3Session    *session.Session
	}
)

func newPartition(namespace string, id int32) (*Partition, error) {
	db, restored, err := createLocalDB(id)
	if err != nil {
		return nil, fmt.Errorf("error in createLocalDB: %w", err)
	}

	part := &Partition{
		ID:        id,
		DB:        db,
		GCTicker:  time.NewTicker(time.Millisecond * time.Duration(utils.Env_GCIntervalMs)),
		closeChan: make(chan struct{}, 2), // gc and backup
		BackupWg:  sync.WaitGroup{},
		Namespace: namespace,
	}

	if utils.Env_BackupEnabled || utils.Env_S3RestoreEnabled {
		// Other options use default AWS SDK env vars
		logger.Debug().Msgf("configuring s3 session for partition %d", part.ID)
		cfg := &aws.Config{
			Endpoint:         &utils.Env_BackupS3Endpoint,
			DisableSSL:       aws.Bool(strings.Contains(utils.Env_BackupS3Endpoint, "http://")),
			S3ForcePathStyle: aws.Bool(true),
		}
		part.s3Session, err = session.NewSession(cfg)
		if err != nil {
			return nil, fmt.Errorf("error in session.NewSession: %w", err)
		}
	}

	if restored {
		logger.Info().Msgf("partition %d locally restored", id)
		logger.Debug().Msgf("checking last seen time for partition %d", id)
		// Get time if we have it
		var b []byte
		err := db.View(func(txn *badger.Txn) error {
			item, err := txn.Get(LastOffsetKey)
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			b, err = item.ValueCopy(nil)
			if err != nil {
				return fmt.Errorf("error in item.ValueCopy: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("error getting LastOffsetKey: %w", err)
		}
		if len(b) > 0 {
			part.LastOffset = utils.Ptr(int64(binary.LittleEndian.Uint64(b)))
		}
	}

	// Check for remote backup
	if utils.Env_S3RestoreEnabled {
		logger.Debug().Msgf("checking for s3 backups for partition %d", part.ID)
		err := part.checkAndRestoreFromS3()
		if errors.Is(err, ErrBackupNotFound) {
			logger.Debug().Msgf("no remote backup found for partition %d", part.ID)
		} else if errors.Is(err, ErrRemoteOffsetLower) {
			logger.Debug().Msgf("remote backup for partition %d had a lower offset than locally, using local restore", part.ID)
		} else if err != nil {
			return nil, fmt.Errorf("error in checkAndRestoreFromS3: %w", err)
		}
	}

	if utils.Env_BackupEnabled {
		part.BackupTicker = time.NewTicker(time.Second * time.Duration(utils.Env_BackupIntervalSec))
		go part.backupInterval()
	}

	go part.gcTickHandler()

	return part, nil
}

func (p *Partition) gcTickHandler() {
	for {
		select {
		case <-p.GCTicker.C:
			// TODO: check for active backup and wait on channel
			p.BackupWg.Wait()
			logger.Debug().Msgf("running garbage collection for partition %d", p.ID)
			var err error
			for err == nil {
				// suggested by docs
				err = p.DB.RunValueLogGC(0.5)
			}
			if errors.Is(err, badger.ErrNoRewrite) {
				logger.Debug().Msgf("garbage collection has nothing to rewrite for partition %d", p.ID)
				continue
			}
			if err != nil {
				logger.Error().Err(err).Int32("partition", p.ID).Msg("error running garbage collection! Non-fatal error, but the DB might get really big!")
			}
		case <-p.closeChan:
			logger.Debug().Msgf("gc ticker for partition %d received on close channel, exiting", p.ID)
			return
		}
	}
}

func createLocalDB(id int32) (*badger.DB, bool, error) {
	partitionPath := getPartitionPath(id)
	// Check if the DB already exists on disk
	exists := false
	if _, err := os.Stat(partitionPath); err == nil {
		logger.Debug().Msgf("found existing DB file %s", partitionPath)
		exists = true
	}
	opts := badger.DefaultOptions(partitionPath)
	// If we are running into large file sizes, we can try setting NumLevelZeroTables to 0 or 1,
	// and NumLevelZeroTablesStall to 1 or 2 as per https://github.com/dgraph-io/badger/issues/718#issuecomment-467886596
	if !utils.Env_BadgerDebug {
		opts.Logger = nil
	}
	db, err := badger.Open(opts)
	if err != nil {
		return nil, false, fmt.Errorf("error in badger.Open: %w", err)
	}

	logger.Debug().Msgf("created db for partition %s", partitionPath)

	return db, exists, nil
}

func (p *Partition) delete() error {
	err := p.Shutdown()
	if err != nil {
		return fmt.Errorf("error shutting down partition: %w", err)
	}
	logger.Info().Msgf("deleting local partition %d", p.ID)
	return os.RemoveAll(getPartitionPath(p.ID))
}

func (p *Partition) Shutdown() error {
	logger.Debug().Msgf("shutting down partition %d", p.ID)
	p.GCTicker.Stop()
	p.closeChan <- struct{}{} // gc
	p.closeChan <- struct{}{} // backup
	return p.DB.Close()
}

func getPartitionPath(id int32) string {
	return path.Join(utils.Env_DBPath, fmt.Sprintf("%d.db", id))
}

func (p *Partition) ReadRecords(keys []RecordKey) ([]Record, error) {
	var records []Record
	s := time.Now()
	err := p.DB.View(func(txn *badger.Txn) error {
		for _, key := range keys {
			item, err := txn.Get(formatRecordKey(key.Pk, key.Sk))
			if errors.Is(err, badger.ErrKeyNotFound) {
				continue
			}
			if err != nil {
				return fmt.Errorf("error in txn.Get: %w", err)
			}
			b, err := item.ValueCopy(nil)
			if err != nil {
				return fmt.Errorf("error in item.ValueCopy: %w", err)
			}

			var storedRecord StoredRecord
			err = json.Unmarshal(b, &storedRecord)
			if err != nil {
				return fmt.Errorf("error in json.Unmarshal: %w", err)
			}
			records = append(records, storedRecord.Record(key.Pk, key.Sk))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error reading record from DB: %w", err)
	}
	internal.Metric_LocalPartitionLatenciesMicro.With(map[string]string{
		"operation": "get",
	}).Observe(float64(time.Since(s).Microseconds()))
	return records, nil
}

func (p *Partition) HandleMutation(mutation RecordMutation, offset int64) error {
	switch mutation.Mutation {
	case OperationPut:
		return p.handlePut(mutation.Pk, mutation.Sk, mutation.If, mutation.Data, offset, mutation.TsMs)
	case OperationDelete:
		return p.handleDelete(mutation.Pk, mutation.Sk, mutation.If, offset)
	default:
		return ErrUnknownOperation
	}
}

func (p *Partition) handlePut(pk, sk string, ifStmt *string, data map[string]any, offset, tsMs int64) error {
	// TODO: remove log line
	logger.Debug().Msg("handling put")
	key := formatRecordKey(pk, sk)
	s := time.Now()
	n := time.UnixMilli(tsMs)
	err := p.DB.Update(func(txn *badger.Txn) error {
		var stored StoredRecord
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			stored.UpdatedAt = n
			stored.CreatedAt = n
		} else if err != nil {
			return fmt.Errorf("error in txn.Get: %w", err)
		} else {
			b, err := item.ValueCopy(nil)
			if err != nil {
				return fmt.Errorf("error in item.ValueCopy: %w", err)
			}

			err = json.Unmarshal(b, &stored)
			if err != nil {
				return fmt.Errorf("error in json.Unmarshal: %w", err)
			}
			stored.UpdatedAt = n
		}

		shouldUpdate := true
		if ifStmt != nil {
			shouldUpdate, err = CompareIfStatement(*ifStmt, stored.ToCompareRecord(pk, sk))
			if err != nil {
				return fmt.Errorf("error in CompareIfStatement: %w", err)
			}
		}

		if shouldUpdate {
			stored.Data = data
			err = txn.Set(key, stored.MustSerialize())
			if err != nil {
				return fmt.Errorf("error setting record: %w", err)
			}
		}

		return p.updateOffsetInTx(txn, offset)
	})
	if err != nil {
		internal.Metric_LocalPartitionLatenciesMicro.With(map[string]string{
			"operation": "put",
		}).Observe(float64(time.Since(s).Microseconds()))
	}
	return err
}

// updateOffsetInTx handles the error message for you so you can just return it
func (p *Partition) updateOffsetInTx(txn *badger.Txn, offset int64) error {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(offset))
	err := txn.Set(LastOffsetKey, b)
	if err != nil {
		return fmt.Errorf("error in setting offset: %w", err)
	}
	p.LastOffset = &offset
	return nil
}

func (p *Partition) handleDelete(pk, sk string, ifStmt *string, offset int64) error {
	// TODO: remove log line
	logger.Debug().Msg("handling delete")
	s := time.Now()
	key := formatRecordKey(pk, sk)
	err := p.DB.Update(func(txn *badger.Txn) error {
		shouldDelete := true
		if ifStmt != nil {
			var stored StoredRecord
			item, err := txn.Get(key)
			if errors.Is(err, badger.ErrKeyNotFound) {
				// we didn't even have it
				return nil
			} else if err != nil {
				return fmt.Errorf("error in txn.Get: %w", err)
			} else {
				b, err := item.ValueCopy(nil)
				if err != nil {
					return fmt.Errorf("error in item.ValueCopy: %w", err)
				}

				err = json.Unmarshal(b, &stored)
				if err != nil {
					return fmt.Errorf("error in json.Unmarshal: %w", err)
				}
			}
			shouldDelete, err = CompareIfStatement(*ifStmt, stored.ToCompareRecord(pk, sk))
			if err != nil {
				return fmt.Errorf("error in CompareIfStatement: %w", err)
			}
		}

		if shouldDelete {
			err := txn.Delete(formatRecordKey(pk, sk))
			if err != nil {
				return fmt.Errorf("error deleting record: %w", err)
			}
		}

		return p.updateOffsetInTx(txn, offset)
	})
	if err != nil {
		internal.Metric_LocalPartitionLatenciesMicro.With(map[string]string{
			"operation": "delete",
		}).Observe(float64(time.Since(s).Microseconds()))
	}
	return err
}

// pkToStorageKey formats the key for storage
func pkToStorageKey(pk string) string {
	return KeyPrefix + pk
}

// pkFromStorageKey removes the storage key prefix
func pkFromStorageKey(storagePK string) string {
	return storagePK[len(KeyPrefix):]
}

// formatRecordKey takes in a pk and sk and returns the key that is used in the db
func formatRecordKey(pk, sk string) []byte {
	return []byte(pkToStorageKey(pk) + "\x00" + sk)
}

// splitRecordKey will split a record key into it's pk and sk parts
func splitRecordKey(recordKey []byte) (pk string, sk string, err error) {
	parts := strings.Split(string(recordKey), "\x00")
	if len(parts) != 2 {
		return "", "", ErrInvalidKey
	}
	return pkFromStorageKey(parts[0]), parts[1], nil
}
