package chat_pipeline

import (
	"io"
	"testing"
)

func TestRuntimeCloseClosesOwnedResourcesOnce(t *testing.T) {
	first := &runtimeCloser{}
	second := &runtimeCloser{}
	runtime := &Runtime{resources: []io.Closer{first, second}}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if first.closed != 1 || second.closed != 1 {
		t.Fatalf("resource close counts: %d, %d", first.closed, second.closed)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if first.closed != 1 || second.closed != 1 {
		t.Fatal("resources closed more than once")
	}
}

type runtimeCloser struct{ closed int }

func (c *runtimeCloser) Close() error {
	c.closed++
	return nil
}
