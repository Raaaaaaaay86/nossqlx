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
}
