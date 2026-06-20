package nossqlx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ PostgreRunner = (*pgSmartRunner)(nil)

// pgSmartRunner routes writes to master and reads to replica.
// Falls back to master when replica is nil.
type pgSmartRunner struct {
	master  *pgxpool.Pool
	replica *pgxpool.Pool
}

func (r *pgSmartRunner) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return r.master.Exec(ctx, sql, arguments...)
}

func (r *pgSmartRunner) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	return r.readPool().Query(ctx, query, args...)
}

func (r *pgSmartRunner) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return r.readPool().QueryRow(ctx, sql, args...)
}

func (r *pgSmartRunner) readPool() *pgxpool.Pool {
	if r.replica != nil {
		return r.replica
	}
	return r.master
}
