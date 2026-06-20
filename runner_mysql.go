package nossqlx

import (
	"context"
	stdsql "database/sql"

	"github.com/jmoiron/sqlx"
)

var _ MySQLRunner = (*mysqlSmartRunner)(nil)

// mysqlSmartRunner routes writes to master and reads to replica.
// Falls back to master when replica is nil.
type mysqlSmartRunner struct {
	master  *sqlx.DB
	replica *sqlx.DB
}

func (r *mysqlSmartRunner) ExecContext(ctx context.Context, query string, arguments ...any) (stdsql.Result, error) {
	return r.master.ExecContext(ctx, query, arguments...)
}

func (r *mysqlSmartRunner) PrepareContext(ctx context.Context, query string) (*stdsql.Stmt, error) {
	return r.master.PrepareContext(ctx, query)
}

func (r *mysqlSmartRunner) QueryContext(ctx context.Context, query string, args ...any) (*stdsql.Rows, error) {
	return r.readDB().QueryContext(ctx, query, args...)
}

func (r *mysqlSmartRunner) QueryRowContext(ctx context.Context, query string, args ...any) *stdsql.Row {
	return r.readDB().QueryRowContext(ctx, query, args...)
}

func (r *mysqlSmartRunner) readDB() *sqlx.DB {
	if r.replica != nil {
		return r.replica
	}
	return r.master
}
