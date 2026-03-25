package nossqlx

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/xerrors"
)

type transactionCtx struct{}

type Transaction struct {
	Level    int
	Commit   func(context.Context) error
	Rollback func(context.Context) error
	StartAt  time.Time
	Lock     sync.Mutex
	Tx       any
}

func BeginTx(ctx context.Context, fn func(ctx context.Context) error) error {
	transaction, ok := ctx.Value(transactionCtx{}).(*Transaction)
	if !ok {
		transaction = &Transaction{StartAt: time.Now()}
	} else {
		transaction = &Transaction{
			Level:   transaction.Level + 1,
			StartAt: time.Now(),
		}
	}

	ctx = context.WithValue(ctx, transactionCtx{}, transaction)

	if err := fn(ctx); err != nil {
		if transaction.Rollback != nil {
			if rollbackErr := transaction.Rollback(ctx); rollbackErr != nil {
				slog.Error("transaction rollback failed", "error", rollbackErr)
			} else {
				slog.Debug("transaction rollback succeed", "error", err)
			}
		} else {
			slog.Error("transaction failed and has no rollback callback function", "error", err)
		}

		return xerrors.Errorf("transaction failed: %w", err)
	}

	if transaction.Commit != nil {
		if err := transaction.Commit(ctx); err != nil {
			slog.Error("transaction commit failed", "error", err)
			return err
		} else {
			slog.Debug("transaction commit")
		}
	} else {
		slog.Error("transaction has no rollback callback function")

		return xerrors.Errorf("missing commit callback")
	}

	return nil
}
