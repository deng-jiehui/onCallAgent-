package chat_pipeline

import (
	"errors"
	"strings"
	"testing"
)

func TestAddGraphStepIncludesStageAndName(t *testing.T) {
	sentinel := errors.New("duplicate node")
	err := addGraphStep("node", "ReactAgent", func() error { return sentinel })
	if err == nil {
		t.Fatal("expected graph construction error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("wrapped error does not preserve cause: %v", err)
	}
	if !strings.Contains(err.Error(), "graph node") || !strings.Contains(err.Error(), "ReactAgent") {
		t.Fatalf("error does not identify failed graph step: %v", err)
	}
}
