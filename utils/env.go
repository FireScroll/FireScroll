package utils

var (
	Env_SelfIP                 = MustEnv("SELF_IP")
	Env_SleepSeconds           = MustEnvOrDefaultInt64("SHUTDOWN_SLEEP_SEC", 0)
	Env_ShutdownTimeoutSeconds = MustEnvOrDefaultInt64("SHUTDOWN_TIMEOUT_SEC", 1)

	Env_Namespace        = MustEnv("NAMESPACE")
	Env_ReplicaGroupName = MustEnv("REPLICA_GROUP")
	Env_KafkaSessionMs   = MustEnvOrDefaultInt64("KAFKA_SESSION_MS", 60_000)
	Env_NumPartitions    = MustEnvOrDefaultInt64("PARTITIONS", 256)
)
