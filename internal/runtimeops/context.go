package runtimeops

import "context"

type ctxKey int

const (
	ctxKeyOperationID ctxKey = iota
	ctxKeyEngine
	ctxKeyMsUUID
)

// OperationMeta holds correlation fields propagated through context.
type OperationMeta struct {
	OperationID string
	Engine      string
	MsUUID      string
}

// WithOperation returns a child context carrying runtime operation correlation fields.
func WithOperation(ctx context.Context, operationID, engine, msUUID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, ctxKeyOperationID, operationID)
	ctx = context.WithValue(ctx, ctxKeyEngine, engine)
	ctx = context.WithValue(ctx, ctxKeyMsUUID, msUUID)
	return ctx
}

// OperationFromContext returns correlation fields stored in ctx (empty strings if unset).
func OperationFromContext(ctx context.Context) OperationMeta {
	if ctx == nil {
		return OperationMeta{}
	}
	meta := OperationMeta{}
	if v, ok := ctx.Value(ctxKeyOperationID).(string); ok {
		meta.OperationID = v
	}
	if v, ok := ctx.Value(ctxKeyEngine).(string); ok {
		meta.Engine = v
	}
	if v, ok := ctx.Value(ctxKeyMsUUID).(string); ok {
		meta.MsUUID = v
	}
	return meta
}
