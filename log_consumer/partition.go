package log_consumer

type (
	// Partition manages a single partition of a log, mapping to a SQLite DB and handling backups.
	// It also manages raft participation.
	Partition struct {
	}
)
