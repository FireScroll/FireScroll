package log_consumer

type (
	// LogConsumer is a single consumer of log, belonging to a single consumer group.
	// It also manages gossip participation
	LogConsumer struct {
		ConsumerGroup string

		// ManagedPartitions are the partitions that are managed on this node
		ManagedPartitions []*Partition
	}
)
