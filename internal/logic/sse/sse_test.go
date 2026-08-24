package sse

import (
	"strings"
	"testing"
)

func TestFormatEvent(t *testing.T) {
	got := formatEvent("message", "hello")
	if !strings.Contains(got, "event: message\ndata: hello\n\n") {
		t.Fatalf("unexpected SSE event: %q", got)
	}
}
