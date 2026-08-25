package chat_pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type adapterTestRunner struct {
	message *schema.Message
	stream  *schema.StreamReader[*schema.Message]
}

func (r *adapterTestRunner) Invoke(context.Context, *UserMessage, ...compose.Option) (*schema.Message, error) {
	return r.message, nil
}

func (r *adapterTestRunner) Stream(context.Context, *UserMessage, ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	return r.stream, nil
}

func (*adapterTestRunner) Collect(context.Context, *schema.StreamReader[*UserMessage], ...compose.Option) (*schema.Message, error) {
	return nil, errors.New("not used")
}

func (*adapterTestRunner) Transform(context.Context, *schema.StreamReader[*UserMessage], ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not used")
}

func TestADKAgentRunConvertsInvokeResult(t *testing.T) {
	runner := &adapterTestRunner{message: schema.AssistantMessage("answer", nil)}
	agent, err := NewADKAgent(runner, "chat", "chat agent")
	if err != nil {
		t.Fatalf("NewADKAgent returned error: %v", err)
	}

	iter := agent.Run(context.Background(), &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("question")},
	})
	event, ok := iter.Next()
	if !ok || event == nil {
		t.Fatal("expected an agent event")
	}
	if event.Err != nil {
		t.Fatalf("agent event returned error: %v", event.Err)
	}
	message, _, err := adk.GetMessage(event)
	if err != nil {
		t.Fatalf("read agent message: %v", err)
	}
	if message.Content != "answer" {
		t.Fatalf("unexpected answer: %q", message.Content)
	}
	if _, ok := iter.Next(); ok {
		t.Fatal("expected agent iterator to close after one result")
	}
}

func TestADKAgentRunConvertsStreamResult(t *testing.T) {
	runner := &adapterTestRunner{
		stream: schema.StreamReaderFromArray([]*schema.Message{
			schema.AssistantMessage("part", nil),
		}),
	}
	agent, err := NewADKAgent(runner, "chat", "chat agent")
	if err != nil {
		t.Fatalf("NewADKAgent returned error: %v", err)
	}

	iter := agent.Run(context.Background(), &adk.AgentInput{
		Messages:        []*schema.Message{schema.UserMessage("question")},
		EnableStreaming: true,
	})
	event, ok := iter.Next()
	if !ok || event == nil || event.Output == nil || event.Output.MessageOutput == nil {
		t.Fatal("expected a streaming agent event")
	}
	if !event.Output.MessageOutput.IsStreaming {
		t.Fatal("expected streaming message variant")
	}
	message, _, err := adk.GetMessage(event)
	if err != nil {
		t.Fatalf("read streamed agent message: %v", err)
	}
	if message.Content != "part" {
		t.Fatalf("unexpected streamed answer: %q", message.Content)
	}
}
