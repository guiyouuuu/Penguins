package requestctx

import "context"

type requestIDKey struct{}

func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
func ID(ctx context.Context) string { id, _ := ctx.Value(requestIDKey{}).(string); return id }
