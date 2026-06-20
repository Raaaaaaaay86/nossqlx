package nossqlx

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	"github.com/uptrace/opentelemetry-go-extra/otelsqlx"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"golang.org/x/xerrors"
)

type MySQLClient struct {
	master     *sqlx.DB
	replicas   []*sqlx.DB
	replicaIdx atomic.Uint64
	sqlTimeout time.Duration
}

func NewSqlxMySQLClient(c ClientConfig) (*MySQLClient, error) {
	useOtel := c.TracerProvider != nil

	masterDSN := mysqlDSN(c.Username, c.Password, c.Host, c.Port, c.Database)
	master, err := conn("mysql", masterDSN, useOtel)
	if err != nil {
		return nil, xerrors.Errorf("connect to master mysql failed: %w", err)
	}

	replicas := make([]*sqlx.DB, 0, len(c.Replicas))
	for i, rc := range c.Replicas {
		username, password := rc.Username, rc.Password
		if username == "" {
			username = c.Username
		}
		if password == "" {
			password = c.Password
		}
		db, err := conn("mysql", mysqlDSN(username, password, rc.Host, rc.Port, c.Database), useOtel)
		if err != nil {
			return nil, xerrors.Errorf("connect to replica[%d] mysql failed: %w", i, err)
		}
		replicas = append(replicas, db)
	}

	return &MySQLClient{
		master:     master,
		replicas:   replicas,
		sqlTimeout: c.SQLTimeout,
	}, nil
}

func mysqlDSN(username, password, host string, port int, database string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", username, password, host, port, database)
}

func conn(driverName string, dsn string, useOtel bool) (*sqlx.DB, error) {
	if !useOtel {
		return sqlx.Connect(driverName, dsn)
	}

	options := []otelsql.Option{
		otelsql.WithAttributes(
			semconv.DBSystemNameMySQL,
			attribute.String("db.system.name", "mysql"),
		),
	}
	return otelsqlx.Connect(driverName, dsn, options...)
}

func (s *MySQLClient) DB() *sqlx.DB {
	return s.master
}

func (s *MySQLClient) Session(c context.Context) (context.Context, context.CancelFunc, MySQLRunner, error) {
	ctx, cancel := context.WithTimeout(c, s.sqlTimeout)
	runner, err := getMySQLConn(ctx, s.master, s.nextReplica())
	return ctx, cancel, runner, err
}

func (s *MySQLClient) nextReplica() *sqlx.DB {
	if len(s.replicas) == 0 {
		return nil
	}

	idx := s.replicaIdx.Add(1) - 1
	return s.replicas[idx%uint64(len(s.replicas))]
}

func getMySQLConn(ctx context.Context, master *sqlx.DB, replica *sqlx.DB) (MySQLRunner, error) {
	if master == nil {
		return nil, xerrors.Errorf("missing database instance *sqlx.DB")
	}

	if ctxValue := ctx.Value(transactionCtx{}); ctxValue != nil {
		transaction, ok := ctxValue.(*Transaction)
		if !ok {
			return nil, xerrors.Errorf("unexpected type: %T", ctxValue)
		}
		return joinOrBeginMySQLTx(transaction, master)
	}

	if routeFromCtx(ctx) == routeMaster {
		return master, nil
	}

	return &mysqlSmartRunner{master: master, replica: replica}, nil
}

func joinOrBeginMySQLTx(transaction *Transaction, db *sqlx.DB) (MySQLRunner, error) {
	transaction.Lock.Lock()
	defer transaction.Lock.Unlock()

	if transaction.Commit == nil {
		tx, err := db.BeginTxx(transaction.rootCtx, nil)
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
