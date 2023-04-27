package utils

var (
	Env_SleepSeconds           = MustEnvOrDefaultInt64("SHUTDOWN_SLEEP_SEC", 0)
	Env_ShutdownTimeoutSeconds = MustEnvOrDefaultInt64("SHUTDOWN_TIMEOUT_SEC", 1)

	Env_Namespace        = MustEnv("NAMESPACE")
	Env_ReplicaGroupName = MustEnv("REPLICA_GROUP")
	Env_InstanceID       = MustEnv("INSTANCE_ID")
	Env_KafkaSessionMs   = MustEnvOrDefaultInt64("KAFKA_SESSION_MS", 60_000)
	Env_NumPartitions    = MustEnvOrDefaultInt64("PARTITIONS", 256)
	Env_TopicRetentionMS = MustEnvInt64("TOPIC_RETENTION_MS")

	Env_DBPath = EnvOrDefault("DB_PATH", "/var/fanoutdb/db")

	Env_APIPort = EnvOrDefault("API_PORT", "8070")
)
