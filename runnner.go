package nossqlx

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type PostgreRunner interface {
	PostgreExecer
	PostgreQueryer
}

type PostgreExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type PostgreQueryer interface {
	Query(ctx context.Context, query string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type MySQLRunner interface {
	MySQLExecer
	MySQLPreparer
	MySQLQueryer
}

type MySQLExecer interface {
	ExecContext(ctx context.Context, sql string, arguments ...any) (sql.Result, error)
}

type MySQLPreparer interface {
	PrepareContext(context.Context, string) (*sql.Stmt, error)
}

type MySQLQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, sql string, args ...any) *sql.Row
}
