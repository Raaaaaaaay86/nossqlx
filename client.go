package nossqlx

import (
	"time"

	"go.opentelemetry.io/otel/trace"
)

type ClientConfig struct {
	Host           string
	Port           int
	Database       string
	Username       string
	Password       string
	SQLTimeout     time.Duration
	TracerProvider trace.TracerProvider
	Replicas       []ReplicaConfig
	Pool           PoolConfig
}

// ReplicaConfig holds connection info for a read replica.
// Username and Password fall back to ClientConfig values when empty.
type ReplicaConfig struct {
	Host     string
	Port     int
	Username string
	Password string
}

// PoolConfig controls connection pool size for both master and replicas.
// Zero values leave the driver default unchanged.
//
// pgxpool mapping: MaxConns→MaxConns, MinConns→MinConns
// database/sql mapping: MaxConns→SetMaxOpenConns, MinConns→SetMaxIdleConns
type PoolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}
