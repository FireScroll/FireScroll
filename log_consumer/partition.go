package log_consumer

type (
	// Partition manages a single partition of a log, mapping to a SQLite DB and handling backups.
	Partition struct {
		ID           int
		BackupLeader bool
	}
)
