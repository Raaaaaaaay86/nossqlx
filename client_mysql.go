package nossqlx

import (
	"context"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	"github.com/uptrace/opentelemetry-go-extra/otelsqlx"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"golang.org/x/xerrors"
)

type MySQLClient struct {
	db         *sqlx.DB
	sqlTimeout time.Duration
}

func NewSqlxMySQLClient(c ClientConfig) (*MySQLClient, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.Username, c.Password, c.Host, c.Port, c.Database)

	db, err := conn("mysql", dsn, c.TracerProvider != nil)
	if err != nil {
		return nil, xerrors.Errorf("connect to mysql failed: %w", err)
	}

	instance := &MySQLClient{
		db:         db,
		sqlTimeout: c.SQLTimeout,
	}

	return instance, nil
}

func conn(driverName string, dsn string, useOtel bool) (*sqlx.DB, error) {
	if !useOtel {
		return sqlx.Connect(driverName, dsn)
	}

	options := []otelsql.Option{
		otelsql.WithAttributes(
			semconv.DBSystemNameMySQL,
		),
	}
	return otelsqlx.Connect(driverName, dsn, options...)
}

func (s *MySQLClient) DB() *sqlx.DB {
	return s.db
}

func (s *MySQLClient) Session(c context.Context) (context.Context, context.CancelFunc, MySQLRunner, error) {
	ctx, cancel := context.WithTimeout(c, s.sqlTimeout)

	runner, err := getMySQLConn(ctx, s.db)

	return ctx, cancel, runner, err
}

func getMySQLConn(ctx context.Context, db *sqlx.DB) (MySQLRunner, error) {
	if db == nil {
		return nil, xerrors.Errorf("missing database instance *sqlx.DB")
	}

	ctxValue := ctx.Value(transactionCtx{})

	if ctxValue == nil {
		return db, nil
	}

	transaction, ok := ctxValue.(*Transaction)
	if !ok {
		return nil, xerrors.Errorf("unexpected type: %t", ctxValue)
	}

	transaction.Lock.Lock()
	defer transaction.Lock.Unlock()
	if transaction.Commit == nil {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			return nil, xerrors.Errorf("begin transaction failed: %w", err)
		}

		transaction.Commit = func(ctx context.Context) error {
			return tx.Commit()
		}
		transaction.Rollback = func(ctx context.Context) error {
			return tx.Rollback()
		}
		transaction.Tx = tx

		return tx, nil
	}

	tx, ok := transaction.Tx.(*sqlx.Tx)
	if !ok {
		return nil, xerrors.Errorf("unexpected transaction type: %T", transaction.Tx)
	}

	return tx, nil
}
