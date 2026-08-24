package observability

import (
	"context"
	"testing"
)

func TestStartRequestEndsExactlyOnce(t *testing.T) {
	recorder := installSpanRecorder(t)
	ctx, finish := StartRequest(context.Background(), "/api/chat", "invoke", "conversation-1")
	info := RequestInfoFromContext(ctx)
	if info.RequestID == "" || info.ConversationID != "conversation-1" {
		t.Fatalf("unexpected request info: %#v", info)
	}
	finish(nil)
	finish(nil)
	span := onlySpan(t, recorder)
	if span.Name() != "HTTP /api/chat" {
		t.Fatalf("unexpected span: %q", span.Name())
	}
}
