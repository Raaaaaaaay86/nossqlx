package nossqlx

import (
	"context"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"golang.org/x/xerrors"
)

type MySQLClient struct {
	db         *sqlx.DB
	sqlTimeout time.Duration
}

func NewSqlxMySQLClient(c ClientConfig) (*MySQLClient, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.Username, c.Password, c.Host, c.Port, c.Database)

	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return nil, xerrors.Errorf("connect to mysql failed: %w", err)
	}

	instance := &MySQLClient{
		db:         db,
		sqlTimeout: c.SQLTimeout,
	}

	return instance, nil
}

func (s *MySQLClient) DB() *sqlx.DB {
	return s.db
}

func (s *MySQLClient) Session(c context.Context) (context.Context, context.CancelFunc, MySQLRunner, func(), error) {
	ctx, cancel := context.WithTimeout(c, s.sqlTimeout)

	runner, release, err := getMySQLConn(ctx, s.db)

	return ctx, cancel, runner, release, err
}

func getMySQLConn(ctx context.Context, db *sqlx.DB) (MySQLRunner, func(), error) {
	if db == nil {
		return nil, func() {}, xerrors.Errorf("missing database instance *sqlx.DB")
	}

	ctxValue := ctx.Value(transactionCtx{})

	if ctxValue == nil {
		conn, err := db.Conn(ctx)
		if err != nil {
			return nil, func() {}, err
		}
		return conn, func() { conn.Close() }, nil
	}

	transaction, ok := ctxValue.(*Transaction)
	if !ok {
		return nil, func() {}, xerrors.Errorf("unexpected type: %t", ctxValue)
	}

	transaction.Lock.Lock()
	defer transaction.Lock.Unlock()
	if transaction.Commit == nil {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			return nil, func() {}, xerrors.Errorf("begin transaction failed: %w", err)
		}

		transaction.Commit = func(ctx context.Context) error {
			return tx.Commit()
		}
		transaction.Rollback = func(ctx context.Context) error {
			return tx.Rollback()
		}
		transaction.Tx = tx

		return tx, func() {}, nil
	}

	tx, ok := transaction.Tx.(*sqlx.Tx)
	if !ok {
		return nil, func() {}, xerrors.Errorf("unexpected transaction type: %T", transaction.Tx)
	}

	return tx, func() {}, nil
}

