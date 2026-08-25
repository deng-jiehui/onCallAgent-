package conversation

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestTurnLoopRegistryCreatesOneSessionPerKey(t *testing.T) {
	registry := NewTurnLoopRegistry(8)
	agent := &registryTestAgent{}
	key := SessionKey{TenantID: "tenant", UserID: "user", ConversationID: "conversation"}

	const callers = 16
	sessions := make([]*TurnLoopSession, callers)
	var creates atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			session, err := registry.GetOrCreate(context.Background(), key, func(context.Context) (adk.Agent, error) {
				creates.Add(1)
				return agent, nil
			})
			if err != nil {
				t.Errorf("GetOrCreate returned error: %v", err)
				return
			}
			sessions[index] = session
		}(i)
	}
	wg.Wait()

	if creates.Load() != 1 {
		t.Fatalf("expected one session factory call, got %d", creates.Load())
	}
	for _, session := range sessions[1:] {
		if session != sessions[0] {
			t.Fatal("same key returned different sessions")
		}
	}
	if err := registry.StopAll(context.Background()); err != nil {
		t.Fatalf("StopAll returned error: %v", err)
	}
}

func TestTurnLoopRegistrySeparatesTenantAndConversationKeys(t *testing.T) {
	registry := NewTurnLoopRegistry(8)
	agent := &registryTestAgent{}
	factory := func(context.Context) (adk.Agent, error) { return agent, nil }

	first, err := registry.GetOrCreate(context.Background(), SessionKey{TenantID: "tenant-a", UserID: "user", ConversationID: "conversation"}, factory)
	if err != nil {
		t.Fatalf("create first session: %v", err)
	}
	second, err := registry.GetOrCreate(context.Background(), SessionKey{TenantID: "tenant-b", UserID: "user", ConversationID: "conversation"}, factory)
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	if first == second {
		t.Fatal("different tenant keys shared a session")
	}
	if registry.Len() != 2 {
		t.Fatalf("expected two sessions, got %d", registry.Len())
	}
	if err := registry.StopAll(context.Background()); err != nil {
		t.Fatalf("StopAll returned error: %v", err)
	}
}

func TestTurnLoopRegistryRecreatesStoppedSession(t *testing.T) {
	registry := NewTurnLoopRegistry(2)
	agent := &registryTestAgent{}
	key := SessionKey{TenantID: "tenant", UserID: "user", ConversationID: "conversation"}
	factory := func(context.Context) (adk.Agent, error) { return agent, nil }

	first, err := registry.GetOrCreate(context.Background(), key, factory)
	if err != nil {
		t.Fatalf("create first session: %v", err)
	}
	if err := first.Stop(context.Background()); err != nil {
		t.Fatalf("stop first session: %v", err)
	}
	second, err := registry.GetOrCreate(context.Background(), key, factory)
	if err != nil {
		t.Fatalf("recreate session: %v", err)
	}
	if first == second {
		t.Fatal("stopped session was reused")
	}
	if second.IsStopped() {
		t.Fatal("new session is already stopped")
	}
	if err := registry.StopAll(context.Background()); err != nil {
		t.Fatalf("stop recreated session: %v", err)
	}
}

type registryTestAgent struct{}

func (*registryTestAgent) Name(context.Context) string { return "registry-test-agent" }

func (*registryTestAgent) Description(context.Context) string { return "registry test agent" }

func (*registryTestAgent) Run(_ context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
		Message: schema.AssistantMessage(input.Messages[len(input.Messages)-1].Content, nil),
	}}})
	gen.Close()
	return iter
}
