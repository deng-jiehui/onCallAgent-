package chat_pipeline

import (
	"context"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// Runtime owns the process-wide, request-independent chat agent graph.
// Request identity and conversation state must remain in the call context and input.
type Runtime struct {
	Agent compose.Runnable[*UserMessage, *schema.Message]
}

// NewRuntime builds the chat graph and its dependencies once during startup.
func NewRuntime(ctx context.Context) (*Runtime, error) {
	agent, err := BuildChatAgent(ctx)
	if err != nil {
		return nil, err
	}
	return &Runtime{Agent: agent}, nil
}
