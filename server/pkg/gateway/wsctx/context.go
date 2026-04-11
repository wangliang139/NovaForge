package wsctx

import "context"

type connIDKey struct{}

func WithConnID(ctx context.Context, connID string) context.Context {
	return context.WithValue(ctx, connIDKey{}, connID)
}

func ConnIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(connIDKey{})
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

