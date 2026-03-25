package nossqlx

import "time"

type ClientConfig struct {
	Host       string
	Port       int
	Database   string
	Username   string
	Password   string
	SQLTimeout time.Duration
}
