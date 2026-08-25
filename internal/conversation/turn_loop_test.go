package conversation

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type turnLoopTestAgent struct {
	active  int32
	max     int32
	started chan struct{}
	release chan struct{}
}

func (a *turnLoopTestAgent) Name(context.Context) string { return "test-agent" }

func (a *turnLoopTestAgent) Description(context.Context) string { return "test agent" }

func (a *turnLoopTestAgent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	active := atomic.AddInt32(&a.active, 1)
	for {
		current := atomic.LoadInt32(&a.max)
		if active <= current || atomic.CompareAndSwapInt32(&a.max, current, active) {
			break
		}
	}
	select {
	case a.started <- struct{}{}:
	default:
	}

	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer atomic.AddInt32(&a.active, -1)
		select {
		case <-a.release:
		case <-ctx.Done():
			gen.Send(&adk.AgentEvent{Err: ctx.Err()})
			gen.Close()
			return
		}
		message := input.Messages[len(input.Messages)-1]
		gen.Send(&adk.AgentEvent{Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{Message: schema.AssistantMessage(message.Content, nil)},
		}})
		gen.Close()
	}()
	return iter
}

func TestTurnLoopSessionSerializesSameConversation(t *testing.T) {
	agent := &turnLoopTestAgent{started: make(chan struct{}, 4), release: make(chan struct{})}
	session := newTestTurnLoopSession(t, "tenant-a", "user-a", "conversation-a", agent)
	defer session.Stop(context.Background())

	firstEvents := make(chan TurnEvent, 2)
	secondEvents := make(chan TurnEvent, 2)
	pushTurn(t, session, "first", firstEvents)
	pushTurn(t, session, "second", secondEvents)
	waitSignal(t, agent.started)
	select {
	case <-agent.started:
		t.Fatal("same conversation started a second turn concurrently")
	case <-time.After(40 * time.Millisecond):
	}
	close(agent.release)

	waitDone(t, firstEvents)
	waitDone(t, secondEvents)
}

func TestTurnLoopSessionsRunDifferentConversationsConcurrently(t *testing.T) {
	agent := &turnLoopTestAgent{started: make(chan struct{}, 4), release: make(chan struct{})}
	first := newTestTurnLoopSession(t, "tenant-a", "user-a", "conversation-a", agent)
	second := newTestTurnLoopSession(t, "tenant-a", "user-a", "conversation-b", agent)
	defer first.Stop(context.Background())
	defer second.Stop(context.Background())

	firstEvents := make(chan TurnEvent, 2)
	secondEvents := make(chan TurnEvent, 2)
	pushTurn(t, first, "first", firstEvents)
	pushTurn(t, second, "second", secondEvents)
	waitSignal(t, agent.started)
	waitSignal(t, agent.started)
	if got := atomic.LoadInt32(&agent.max); got < 2 {
		t.Fatalf("different conversations did not run concurrently, max active=%d", got)
	}
	close(agent.release)
	waitDone(t, firstEvents)
	waitDone(t, secondEvents)
}

func newTestTurnLoopSession(t *testing.T, tenant, user, conversationID string, agent adk.Agent) *TurnLoopSession {
	t.Helper()
	session, err := NewTurnLoopSession(context.Background(), SessionKey{
		TenantID: tenant, UserID: user, ConversationID: conversationID,
	}, agent)
	if err != nil {
		t.Fatalf("NewTurnLoopSession returned error: %v", err)
	}
	return session
}

func pushTurn(t *testing.T, session *TurnLoopSession, question string, events chan<- TurnEvent) {
	t.Helper()
	err := session.Push(context.Background(), &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage(question)},
	}, func(event TurnEvent) error {
		events <- event
		return nil
	})
	if err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
}

func waitSignal(t *testing.T, signals <-chan struct{}) {
	t.Helper()
	select {
	case <-signals:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for agent turn")
	}
}

func waitDone(t *testing.T, events <-chan TurnEvent) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-events:
			if event.Err != nil {
				t.Fatalf("turn returned error: %v", event.Err)
			}
			if event.Done {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for turn result")
		}
	}
}
