package sse

import (
	"context"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
)

func TestFormatEvent(t *testing.T) {
	got := formatEvent("message", "hello")
	if !strings.Contains(got, "event: message\ndata: hello\n\n") {
		t.Fatalf("unexpected SSE event: %q", got)
	}
}

func TestClientSendIsBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &Client{
		Request:     &ghttp.Request{},
		messageChan: make(chan string, 1),
		ctx:         ctx,
		cancel:      cancel,
	}
	if !client.SendToClient("message", "one") {
		t.Fatal("first event should be accepted")
	}
	if client.SendToClient("message", "two") {
		t.Fatal("full queue should reject the event")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("full queue should cancel the client")
	}
}
