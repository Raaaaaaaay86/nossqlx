package nossqlx

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"golang.org/x/xerrors"
)

type PostgreClient struct {
	pool       *pgxpool.Pool
	sqlTimeout time.Duration
}

func NewSqlxPostgreClient(c ClientConfig) (*PostgreClient, error) {
	address := fmt.Sprintf("postgres://%s:%s@%s:%d/%s", c.Username, c.Password, c.Host, c.Port, c.Database)

	config, err := pgxpool.ParseConfig(address)
	if err != nil {
		return nil, xerrors.Errorf("parse postgresql config failed: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.TODO(), config)
	if err != nil {
		return nil, xerrors.Errorf("create pgxpool failed: %w", err)
	}

	instance := &PostgreClient{
		pool:       pool,
		sqlTimeout: c.SQLTimeout,
	}

	return instance, nil
}

func (s *PostgreClient) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *PostgreClient) Session(c context.Context) (context.Context, context.CancelFunc, PostgreRunner, error) {
	ctx, cancel := context.WithTimeout(c, s.sqlTimeout)

	runner, err := getPgConn(ctx, s.pool)

	return ctx, cancel, runner, err
}

func getPgConn(ctx context.Context, pool *pgxpool.Pool) (PostgreRunner, error) {
	if pool == nil {
		return nil, xerrors.Errorf("missing database instance *pgxpool.Pool")
	}

	ctxValue := ctx.Value(transactionCtx{})

	if ctxValue == nil {
		return pool, nil
	}

	transaction, ok := ctxValue.(*Transaction)
	if !ok {
		return nil, xerrors.Errorf("unexpected type: %t", ctxValue)
	}

	transaction.Lock.Lock()
	defer transaction.Lock.Unlock()
	if transaction.Commit == nil {
		sqlxTx, err := pool.Begin(ctx)
		if err != nil {
			return nil, xerrors.Errorf("begin transaction failed: %w", err)
		}

		transaction.Commit = func(ctx context.Context) error {
			return sqlxTx.Commit(ctx)
		}
		transaction.Rollback = func(ctx context.Context) error {
			return sqlxTx.Rollback(ctx)
		}
		transaction.Tx = sqlxTx

		return sqlxTx, nil
	}

	tx, ok := transaction.Tx.(pgx.Tx)
	if !ok {
		return nil, nil
	}

	return tx, nil
}
