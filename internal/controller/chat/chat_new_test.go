package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	authn "SuperBizAgent/internal/auth"
	"SuperBizAgent/internal/conversation"
	"context"
	"strconv"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
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

func TestNewV1WithStoreUsesInjectedConversationStore(t *testing.T) {
	store := conversation.NewMemoryStore(8)
	controller, ok := NewV1WithStore(&chat_pipeline.Runtime{}, store).(*ControllerV1)
	if !ok {
		t.Fatal("NewV1WithStore should return a ControllerV1")
	}
	if controller.conversations != store {
		t.Fatal("controller must retain the injected conversation store")
	}
	if err := controller.Close(context.Background()); err != nil {
		t.Fatalf("close controller: %v", err)
	}
}

func TestControllerAcquiresOptionalConversationLock(t *testing.T) {
	locker := &testConversationLocker{}
	controller := &ControllerV1{locker: locker}
	key := conversation.SessionKey{TenantID: "tenant", UserID: "user", ConversationID: "conversation"}
	_, release, err := controller.lockTurn(context.Background(), key)
	if err != nil {
		t.Fatalf("lockTurn returned error: %v", err)
	}
	if locker.locks != 1 {
		t.Fatalf("lock calls=%d, want 1", locker.locks)
	}
	release()
	if locker.unlocks != 1 {
		t.Fatalf("unlock calls=%d, want 1", locker.unlocks)
	}
}

type testConversationLocker struct {
	locks   int
	unlocks int
}

func (l *testConversationLocker) Lock(context.Context, conversation.SessionKey) (conversation.ConversationLease, error) {
	l.locks++
	return &testConversationLease{owner: l}, nil
}

type testConversationLease struct{ owner *testConversationLocker }

func (l *testConversationLease) Renew(context.Context) error { return nil }
func (l *testConversationLease) Unlock(context.Context) error {
	l.owner.unlocks++
	return nil
}
func (l *testConversationLease) StartRenewal(context.Context) <-chan error {
	return make(chan error)
}

func TestNewV1InitializesTurnLoopRegistry(t *testing.T) {
	controller, ok := NewV1(&chat_pipeline.Runtime{}).(*ControllerV1)
	if !ok {
		t.Fatal("NewV1 should return a ControllerV1")
	}
	if controller.loops == nil {
		t.Fatal("controller must initialize a TurnLoop registry")
	}
}

func TestChatTurnLoopLoadsHistoryBeforeEachTurn(t *testing.T) {
	controller := &ControllerV1{
		conversations: conversation.NewMemoryStore(8),
		loops:         conversation.NewTurnLoopRegistry(8),
		runtime:       &chat_pipeline.Runtime{ADKAgent: &controllerTestAgent{}},
	}
	ctx := authn.WithPrincipal(context.Background(), authn.Principal{TenantID: "tenant", UserID: "user"})

	first, err := controller.Chat(ctx, &v1.ChatReq{Id: "conversation", Question: "first"})
	if err != nil {
		t.Fatalf("first Chat returned error: %v", err)
	}
	if first.Answer != "answer:first:history=0" {
		t.Fatalf("unexpected first answer: %q", first.Answer)
	}
	second, err := controller.Chat(ctx, &v1.ChatReq{Id: "conversation", Question: "second"})
	if err != nil {
		t.Fatalf("second Chat returned error: %v", err)
	}
	if second.Answer != "answer:second:history=2" {
		t.Fatalf("unexpected second answer: %q", second.Answer)
	}

	key := conversation.SessionKey{TenantID: "tenant", UserID: "user", ConversationID: "conversation"}
	messages, version, err := controller.conversations.Load(ctx, key)
	if err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if version != 2 || len(messages) != 4 {
		t.Fatalf("expected two committed turns, version=%d messages=%d", version, len(messages))
	}
	if err := controller.Close(context.Background()); err != nil {
		t.Fatalf("close controller: %v", err)
	}
	if controller.loops.Len() != 0 {
		t.Fatal("controller close must stop and remove all turn loops")
	}
}

type controllerTestAgent struct{}

func (*controllerTestAgent) Name(context.Context) string { return "controller-test-agent" }

func (*controllerTestAgent) Description(context.Context) string { return "controller test agent" }

func (*controllerTestAgent) Run(_ context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	last := input.Messages[len(input.Messages)-1]
	answer := "answer:" + last.Content + ":history=" + strconv.Itoa(len(input.Messages)-1)
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
		Message: schema.AssistantMessage(answer, nil),
	}}})
	gen.Close()
	return iter
}
