package partitions

import (
	"database/sql"
	_ "embed"
	"fmt"
	"github.com/danthegoodman1/FanoutDB/utils"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path"
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
		rows, err := db.Query(`select t from offset_keeper where id = 1`) // not worth preparing this since it runs once
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
