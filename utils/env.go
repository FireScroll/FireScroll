package utils

import "os"

var (
	Env_SleepSeconds = MustEnvOrDefaultInt64("SHUTDOWN_SLEEP_SEC", 0)

	Env_Namespace        = MustEnv("NAMESPACE")
	Env_ReplicaGroupName = MustEnv("REPLICA_GROUP")
	Env_Region           = MustEnv("REGION")
	Env_InstanceID       = MustEnv("INSTANCE_ID")
	Env_KafkaSessionMs   = MustEnvOrDefaultInt64("KAFKA_SESSION_MS", 60_000)
	Env_NumPartitions    = MustEnvInt64("PARTITIONS")
	Env_KafkaSeeds       = MustEnv("KAFKA_SEEDS")
	Env_KafkaUsername    = os.Getenv("KAFKA_USER")
	Env_KafkaPassword    = os.Getenv("KAFKA_PASS")
	Env_KafkaTLS         = os.Getenv("KAFKA_TLS") == "1"

	Env_APIPort       = EnvOrDefault("API_PORT", "8190")
	Env_InternalPort  = EnvOrDefault("INTERNAL_PORT", "8191")
	Env_GossipPort    = MustEnvOrDefaultInt64("GOSSIP_PORT", 8192)
	Env_AdvertiseAddr = os.Getenv("ADVERTISE_ADDR") // API, e.g. localhost:8190
	// csv like localhost:8192,localhost:8292
	Env_GossipPeers       = os.Getenv("GOSSIP_PEERS")
	Env_GossipBroadcastMS = MustEnvOrDefaultInt64("GOSSIP_BROADCAST_MS", 1000)

	Env_GCIntervalMs      = MustEnvOrDefaultInt64("GC_INTERVAL_MS", 60_000*5) // 5 minute default
	Env_DBPath            = EnvOrDefault("DB_PATH", "/var/firescroll/dbs")
	Env_BackupEnabled     = os.Getenv("BACKUP") == "1"
	Env_S3RestoreEnabled  = os.Getenv("S3_RESTORE") == "1"
	Env_BackupIntervalSec = MustEnvOrDefaultInt64("BACKUP_INTERVAL_SEC", 60_000*12) // 12 hour default
	Env_BackupTimeoutSec  = MustEnvOrDefaultInt64("BACKUP_TIMEOUT_SEC", 120)        // 12 hour default
	Env_BackupS3Endpoint  = os.Getenv("S3_ENDPOINT")
	Env_BackupS3Bucket    = os.Getenv("S3_BUCKET")
	// AWS sdk will automatically use this
	Env_BackupS3Region = os.Getenv("AWS_REGION")
	// AWS sdk will automatically use this
	Env_BackupS3AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	// AWS sdk will automatically use this
	Env_BackupS3SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")

	Env_Debug       = os.Getenv("DEBUG") == "1"
	Env_BadgerDebug = os.Getenv("BADGER_DEBUG") == "1"
	Env_GossipDebug = os.Getenv("GOSSIP_DEBUG") == "1"

	Env_Profile = os.Getenv("PROFILE") == "1"
)
