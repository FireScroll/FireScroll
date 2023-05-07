package utils

import (
	"golang.org/x/exp/constraints"
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

// MustEnvInt64 gets env var as int64, exits if not found
func MustEnvInt64(env string) int64 {
	res := os.Getenv(env)
	if res == "" {
		log.Fatalf("missing environment variable %s", env)
	}
	intVar, err := strconv.Atoi(res)
	if err != nil {
		log.Fatalf("failed to convert env var %s to an int", env)
	}
	return int64(intVar)
}

// MustEnv will exit if `env` is not provided
func MustEnv(env string) string {
	res := os.Getenv(env)
	if res == "" {
		log.Fatalf("missing environment variable %s", env)
	}
	return res
}

// extracted from franz-go
func Murmur2(b []byte) uint32 {
	const (
		seed uint32 = 0x9747b28c
		m    uint32 = 0x5bd1e995
		r           = 24
	)
	h := seed ^ uint32(len(b))
	for len(b) >= 4 {
		k := uint32(b[3])<<24 + uint32(b[2])<<16 + uint32(b[1])<<8 + uint32(b[0])
		b = b[4:]
		k *= m
		k ^= k >> r
		k *= m

		h *= m
		h ^= k
	}
	switch len(b) {
	case 3:
		h ^= uint32(b[2]) << 16
		fallthrough
	case 2:
		h ^= uint32(b[1]) << 8
		fallthrough
	case 1:
		h ^= uint32(b[0])
		h *= m
	}

	h ^= h >> 13
	h *= m
	h ^= h >> 15
	return h
}

func GetPartition(k string) int32 {
	return int32(Murmur2([]byte(k)) % uint32(Env_NumPartitions))
}

func Ptr[T any](t T) *T {
	return &t
}

func Min[T constraints.Ordered](a, b T) T {
	if a > b {
		return b
	}
	return a
}

func Deref[T any](ref *T, fallback T) T {
	if ref == nil {
		return fallback
	}
	return *ref
}
