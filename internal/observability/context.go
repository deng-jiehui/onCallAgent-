package observability

import "context"

type RequestInfo struct {
	RequestID      string
	ConversationID string
	TenantID       string
	UserID         string
}

type requestInfoContextKey struct{}

func WithRequestInfo(ctx context.Context, info RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoContextKey{}, info)
}

func RequestInfoFromContext(ctx context.Context) RequestInfo {
	info, _ := ctx.Value(requestInfoContextKey{}).(RequestInfo)
	return info
}
