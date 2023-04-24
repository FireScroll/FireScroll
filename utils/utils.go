package utils

import (
	"log"
	"os"
	"strconv"
)

func EnvOrDefault(env, defaultVal string) string {
	if res := os.Getenv(env); res != "" {
		return res
	}
	return defaultVal
}

// MustEnvOrDefaultInt64 will get an env var as an int, exiting if conversion fails
func MustEnvOrDefaultInt64(env string, defaultVal int64) int64 {
	res := os.Getenv(env)
	if res == "" {
		return defaultVal
	}
	// Try to convert to int
	intVar, err := strconv.Atoi(res)
	if err != nil {
		log.Fatalf("failed to convert env var %s to an int", env)
	}
	return int64(intVar)
}
