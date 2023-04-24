package utils

import "os"

var (
	Env_SelfIP                 = os.Getenv("SELF_IP")
	Env_SleepSeconds           = MustEnvOrDefaultInt64("SHUTDOWN_SLEEP_SEC", 0)
	Env_ShutdownTimeoutSeconds = MustEnvOrDefaultInt64("SHUTDOWN_TIMEOUT_SEC", 1)
)
