package ssadb

import "context"

type queryErrorHandlerKey struct{}

// WithQueryErrorHandler lets a scan surface database failures despite the
// legacy channel-only search API. The handler may cancel the owning rule;
// it must be safe for concurrent calls.
func WithQueryErrorHandler(ctx context.Context, handler func(error)) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, queryErrorHandlerKey{}, handler)
}

func reportQueryError(ctx context.Context, err error) {
	if err == nil || ctx == nil {
		return
	}
	if handler, ok := ctx.Value(queryErrorHandlerKey{}).(func(error)); ok && handler != nil {
		handler(err)
	}
}
