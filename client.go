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
}

// ReplicaConfig holds connection info for a read replica.
// Username and Password fall back to ClientConfig values when empty.
type ReplicaConfig struct {
	Host     string
	Port     int
	Username string
	Password string
}
