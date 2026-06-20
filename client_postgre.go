package nossqlx

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"golang.org/x/xerrors"
)

type PostgreClient struct {
	master     *pgxpool.Pool
	replicas   []*pgxpool.Pool
	replicaIdx atomic.Uint64
	sqlTimeout time.Duration
}

func NewSqlxPostgreClient(c ClientConfig) (*PostgreClient, error) {
	master, err := newPgPool(c.Username, c.Password, c.Host, c.Port, c.Database)
	if err != nil {
		return nil, xerrors.Errorf("connect to master postgresql failed: %w", err)
	}

	replicas := make([]*pgxpool.Pool, 0, len(c.Replicas))
	for i, rc := range c.Replicas {
		username, password := rc.Username, rc.Password
		if username == "" {
			username = c.Username
		}
		if password == "" {
			password = c.Password
		}
		pool, err := newPgPool(username, password, rc.Host, rc.Port, c.Database)
		if err != nil {
			return nil, xerrors.Errorf("connect to replica[%d] postgresql failed: %w", i, err)
		}
		replicas = append(replicas, pool)
	}

	return &PostgreClient{
		master:     master,
		replicas:   replicas,
		sqlTimeout: c.SQLTimeout,
	}, nil
}

func newPgPool(username, password, host string, port int, database string) (*pgxpool.Pool, error) {
	address := fmt.Sprintf("postgres://%s:%s@%s:%d/%s", username, password, host, port, database)
	config, err := pgxpool.ParseConfig(address)
	if err != nil {
		return nil, xerrors.Errorf("parse postgresql config failed: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(context.TODO(), config)
	if err != nil {
		return nil, xerrors.Errorf("create pgxpool failed: %w", err)
	}
	return pool, nil
}

func (s *PostgreClient) Pool() *pgxpool.Pool {
	return s.master
}

func (s *PostgreClient) Session(c context.Context) (context.Context, context.CancelFunc, PostgreRunner, error) {
	ctx, cancel := context.WithTimeout(c, s.sqlTimeout)
	runner, err := getPgConn(ctx, s.master, s.nextReplica())
	return ctx, cancel, runner, err
}

func (s *PostgreClient) nextReplica() *pgxpool.Pool {
	if len(s.replicas) == 0 {
		return nil
	}
	idx := s.replicaIdx.Add(1) - 1
	return s.replicas[idx%uint64(len(s.replicas))]
}

func getPgConn(ctx context.Context, master *pgxpool.Pool, replica *pgxpool.Pool) (PostgreRunner, error) {
	if master == nil {
		return nil, xerrors.Errorf("missing database instance *pgxpool.Pool")
	}

	if ctxValue := ctx.Value(transactionCtx{}); ctxValue != nil {
		transaction, ok := ctxValue.(*Transaction)
		if !ok {
			return nil, xerrors.Errorf("unexpected type: %T", ctxValue)
		}
		return joinOrBeginPgTx(transaction, master)
	}

	if routeFromCtx(ctx) == routeMaster {
		return master, nil
	}

	return &pgSmartRunner{master: master, replica: replica}, nil
}

func joinOrBeginPgTx(transaction *Transaction, pool *pgxpool.Pool) (PostgreRunner, error) {
	transaction.Lock.Lock()
	defer transaction.Lock.Unlock()

	if transaction.Commit == nil {
		sqlxTx, err := pool.Begin(transaction.rootCtx)
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
		return nil, xerrors.Errorf("unexpected transaction type: %T", transaction.Tx)
	}
	return tx, nil
}
