package partitions

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/danthegoodman1/Firescroll/utils"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path"
	"strings"
)

var (
	//go:embed create_kv.sql
	CreateKVTableStmt string

	//go:embed create_offset_keeper.sql
	CreateOffsetKeeperTableStmt string
)

type (
	Partition struct {
		ID         int32
		DB         *sql.DB
		LastOffset int64
	}
)

func newPartition(id int32) (*Partition, error) {
	db, restored, err := createLocalDB(id)
	if err != nil {
		return nil, fmt.Errorf("error in createLocalDB: %w", err)
	}

	part := &Partition{
		ID: id,
		DB: db,
	}

	if restored {
		logger.Info().Msgf("partition %d restored", id)
		logger.Debug().Msgf("checking last seen time for partition %d", id)
		// Get time if we have it
		// TODO: for backups, if restored, compare the time of the remote snapshot
		rows, err := db.Query(`select offset from offset_keeper where id = 1`) // not worth preparing this since it runs once
		if err != nil {
			return nil, fmt.Errorf("error querying for offset_keeper time: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			err = rows.Scan(&part.LastOffset)
			if err != nil {
				return nil, fmt.Errorf("error scanning offset_keeper row: %w", err)
			}
		}
		// TODO: remove log line
		if part.LastOffset != 0 {
			logger.Debug().Msgf("partition %d restoring at offset %d", id, part.LastOffset)
		}
	}

	return part, nil
}

func createLocalDB(id int32) (*sql.DB, bool, error) {
	partitionPath := getPartitionPath(id)
	// Check if the DB already exists on disk
	exists := false
	if _, err := os.Stat(partitionPath); err == nil {
		logger.Debug().Msgf("found existing DB file %s", partitionPath)
		exists = true
	}

	db, err := sql.Open("sqlite3", partitionPath+fmt.Sprintf("?_journal_mode=WAL&cache=shared"))
	if err != nil {
		return nil, false, fmt.Errorf("error in sql.Open: %w", err)
	}

	db.SetMaxOpenConns(1)

	if !exists {
		logger.Debug().Msgf("creating tables for partition %d", id)
		_, err = db.Exec(CreateKVTableStmt)
		if err != nil {
			return nil, false, fmt.Errorf("error creating KV table: %w", err)
		}
		_, err = db.Exec(CreateOffsetKeeperTableStmt)
		if err != nil {
			return nil, false, fmt.Errorf("error creating offset keeper table: %w", err)
		}
	}

	logger.Debug().Msgf("created db for partition %s", partitionPath)

	return db, exists, nil
}

func (p *Partition) Shutdown() error {
	// TODO: remove log line?
	logger.Debug().Msgf("shutting down partition %d", p.ID)
	return p.DB.Close()
}

func getPartitionPath(id int32) string {
	return path.Join(utils.Env_DBPath, fmt.Sprintf("%d.db", id))
}

func (p *Partition) ReadRecords(ctx context.Context, keys []RecordKey) ([]Record, error) {
	// Get unique pks and sks for query
	// TODO: VERY TEMP
	var records []Record

	var placeHolders []string
	var flatKeys []any
	for _, key := range keys {
		placeHolders = append(placeHolders, "(pk = ? AND sk = ?)")
		flatKeys = append(flatKeys, key.Pk, key.Sk)
	}
	stmt := fmt.Sprintf(`
		select pk, sk, data, _created_at, _updated_at
		from kv where %s`, strings.Join(placeHolders, " OR "))

	// TODO: remove log line
	logger.Debug().Msgf("running read query: %s", stmt)

	rows, err := p.DB.QueryContext(ctx, stmt, flatKeys...)
	if err != nil {
		return nil, fmt.Errorf("error in DB.Query: %w", err)
	}
	for rows.Next() {
		record := Record{}
		var jsonString string
		err = rows.Scan(&record.Pk, &record.Sk, &jsonString, &record.CreatedAt, &record.UpdatedAt)
		if err != nil {
			return records, fmt.Errorf("error in rows.Scan: %w", err)
		}
		err = json.Unmarshal([]byte(jsonString), &record.Data)
		if err != nil {
			return records, fmt.Errorf("error in json.Unmarshal: %w", err)
		}
		records = append(records, record)
	}

	return records, nil
}

func (p *Partition) HandleMutation(mutation RecordMutation, offset int64) error {
	switch mutation.Mutation {
	case OperationPut:
		b, err := json.Marshal(mutation.Data)
		if err != nil {
			return fmt.Errorf("error in json.Marshal: %w", err)
		}
		return p.handlePut(mutation.Pk, mutation.Sk, b, offset)
	case OperationDelete:
		return p.handleDelete(mutation.Pk, mutation.Sk, offset)
	default:
		return ErrUnknownOperation
	}
}

func (p *Partition) handlePut(pk, sk string, data []byte, offset int64) error {
	// TODO: remove log line
	logger.Debug().Msg("handling put")
	_, err := p.DB.Exec(`insert into kv (pk, sk, data)
		values (?, ?, ?)
		on conflict do update
		set data = excluded.data`, pk, sk, data)
	if err != nil {
		return fmt.Errorf("error in insert: %w", err)
	}
	_, err = p.DB.Exec(`insert into offset_keeper (id, offset)
		values (1, ?)
		on conflict do update
		set offset = excluded.offset`, offset+1) // offset is inclusive
	if err != nil {
		return fmt.Errorf("error in offset update: %w", err)
	}
	return nil
}

func (p *Partition) handleDelete(pk, sk string, offset int64) error {
	// TODO: remove log line
	logger.Debug().Msg("handling delete")
	_, err := p.DB.Exec(`delete from kv where pk = ? and sk = ?`, pk, sk)
	if err != nil {
		return fmt.Errorf("error in delete: %w", err)
	}
	_, err = p.DB.Exec(`insert into offset_keeper (id, offset)
		values (1, ?)
		on conflict do update
		set offset = excluded.offset`, offset+1) // offset is inclusive
	if err != nil {
		return fmt.Errorf("error in offset update: %w", err)
	}
	return nil
}
