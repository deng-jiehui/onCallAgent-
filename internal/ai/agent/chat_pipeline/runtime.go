package chat_pipeline

import (
	"context"
	"io"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// Runtime owns the process-wide, request-independent chat agent graph.
// Request identity and conversation state must remain in the call context and input.
type Runtime struct {
	Agent     compose.Runnable[*UserMessage, *schema.Message]
	ADKAgent  adk.Agent
	resources []io.Closer
}

// NewRuntime builds the chat graph and its dependencies once during startup.
func NewRuntime(ctx context.Context) (*Runtime, error) {
	agent, graphCloser, err := BuildChatAgentWithResources(ctx)
	if err != nil {
		return nil, err
	}
	adkAgent, err := NewADKAgent(agent, "ChatAgent", "Multi-user incident assistance chat agent")
	if err != nil {
		if graphCloser != nil {
			_ = graphCloser.Close()
		}
		return nil, err
	}
	return &Runtime{Agent: agent, ADKAgent: adkAgent, resources: []io.Closer{graphCloser}}, nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var firstErr error
	for i := len(r.resources) - 1; i >= 0; i-- {
		if r.resources[i] == nil {
			continue
		}
		if err := r.resources[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.resources = nil
	return firstErr
}
