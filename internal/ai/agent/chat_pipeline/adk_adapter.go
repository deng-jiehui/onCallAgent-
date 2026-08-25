package chat_pipeline

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ADKAgent adapts the existing compose chat graph to Eino's v0.9 Agent API.
// It translates only the message/event protocol; request identity remains in context.
type ADKAgent struct {
	runner      compose.Runnable[*UserMessage, *schema.Message]
	name        string
	description string
}

func NewADKAgent(runner compose.Runnable[*UserMessage, *schema.Message], name, description string) (*ADKAgent, error) {
	if runner == nil {
		return nil, errors.New("chat agent runner is required")
	}
	if name == "" {
		return nil, errors.New("chat agent name is required")
	}
	return &ADKAgent{runner: runner, name: name, description: description}, nil
}

func (a *ADKAgent) Name(context.Context) string { return a.name }

func (a *ADKAgent) Description(context.Context) string { return a.description }

func (a *ADKAgent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer gen.Close()

		userMessage, err := userMessageFromADKInput(input)
		if err != nil {
			gen.Send(&adk.AgentEvent{AgentName: a.name, Err: err})
			return
		}

		if input.EnableStreaming {
			stream, streamErr := a.runner.Stream(ctx, userMessage)
			if streamErr != nil {
				gen.Send(&adk.AgentEvent{AgentName: a.name, Err: streamErr})
				return
			}
			if stream == nil {
				gen.Send(&adk.AgentEvent{AgentName: a.name, Err: errors.New("chat agent returned nil stream")})
				return
			}
			stream.SetAutomaticClose()
			gen.Send(&adk.AgentEvent{
				AgentName: a.name,
				Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
					IsStreaming:   true,
					MessageStream: stream,
				}},
			})
			return
		}

		message, invokeErr := a.runner.Invoke(ctx, userMessage)
		if invokeErr != nil {
			gen.Send(&adk.AgentEvent{AgentName: a.name, Err: invokeErr})
			return
		}
		if message == nil {
			gen.Send(&adk.AgentEvent{AgentName: a.name, Err: errors.New("chat agent returned nil message")})
			return
		}
		gen.Send(&adk.AgentEvent{
			AgentName: a.name,
			Output:    &adk.AgentOutput{MessageOutput: &adk.MessageVariant{Message: message}},
		})
	}()
	return iter
}

func userMessageFromADKInput(input *adk.AgentInput) (*UserMessage, error) {
	if input == nil || len(input.Messages) == 0 {
		return nil, errors.New("chat agent input messages are required")
	}
	lastUser := -1
	for i := len(input.Messages) - 1; i >= 0; i-- {
		if input.Messages[i] != nil && input.Messages[i].Role == schema.User {
			lastUser = i
			break
		}
	}
	if lastUser < 0 {
		return nil, errors.New("chat agent input requires a user message")
	}
	query := input.Messages[lastUser].Content
	return &UserMessage{
		Query:   query,
		History: cloneMessagesForADK(input.Messages[:lastUser]),
	}, nil
}

func cloneMessagesForADK(messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return nil
	}
	result := make([]*schema.Message, len(messages))
	copy(result, messages)
	return result
}

var _ adk.Agent = (*ADKAgent)(nil)
