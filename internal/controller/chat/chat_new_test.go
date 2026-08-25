package chat

import (
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"testing"
)

func TestNewV1UsesInjectedRuntime(t *testing.T) {
	runtime := &chat_pipeline.Runtime{}
	controller, ok := NewV1(runtime).(*ControllerV1)
	if !ok {
		t.Fatal("NewV1 should return a ControllerV1")
	}
	if controller.runtime != runtime {
		t.Fatal("controller must retain the injected runtime")
	}
}
