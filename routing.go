package nossqlx

import "context"

type routeCtx struct{}

type routeHint int

const (
	routeAuto    routeHint = 0
	routeMaster  routeHint = 1
	routeReplica routeHint = 2
)

func routeFromCtx(ctx context.Context) routeHint {
	hint, _ := ctx.Value(routeCtx{}).(routeHint)
	return hint
}

// ForceMaster forces all Session() calls within fn to route to the primary (master) connection,
// even when replicas are configured. Transactions always use master regardless.
func ForceMaster(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(context.WithValue(ctx, routeCtx{}, routeMaster))
}

// ForceReplica forces all Session() calls within fn to route to a replica connection.
// Falls back to master when no replicas are configured.
// If a transaction is active in the context, master is always used regardless of this hint.
func ForceReplica(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(context.WithValue(ctx, routeCtx{}, routeReplica))
}
