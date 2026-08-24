package conversation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestMemoryStoreIsolatesUsersWithSameConversationID(t *testing.T) {
	store := NewMemoryStore(6)
	keyA := SessionKey{TenantID: "tenant-a", UserID: "user-a", ConversationID: "same-id"}
	keyB := SessionKey{TenantID: "tenant-a", UserID: "user-b", ConversationID: "same-id"}

	if err := store.Append(context.Background(), keyA, schema.UserMessage("A question"), schema.AssistantMessage("A answer", nil)); err != nil {
		t.Fatal(err)
	}
	messagesA, versionA, err := store.Load(context.Background(), keyA)
	if err != nil {
		t.Fatal(err)
	}
	messagesB, versionB, err := store.Load(context.Background(), keyB)
	if err != nil {
		t.Fatal(err)
	}
	if len(messagesA) != 2 || versionA != 1 {
		t.Fatalf("user A history = %d messages, version %d", len(messagesA), versionA)
	}
	if len(messagesB) != 0 || versionB != 0 {
		t.Fatalf("user B saw user A history: %d messages, version %d", len(messagesB), versionB)
	}
}

func TestMemoryStoreLoadReturnsSnapshot(t *testing.T) {
	store := NewMemoryStore(6)
	key := SessionKey{TenantID: "tenant-a", UserID: "user-a", ConversationID: "conv"}
	if err := store.Append(context.Background(), key, schema.UserMessage("question"), schema.AssistantMessage("answer", nil)); err != nil {
		t.Fatal(err)
	}

	snapshot, _, err := store.Load(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	snapshot[0].Content = "mutated outside store"
	snapshot = append(snapshot, schema.UserMessage("extra"))

	again, _, err := store.Load(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Content != "question" || len(again) != 2 {
		t.Fatalf("store was mutated through snapshot: %#v", again)
	}
}

func TestMemoryStoreTrimsCompleteMessagePairs(t *testing.T) {
	store := NewMemoryStore(4)
	key := SessionKey{TenantID: "tenant-a", UserID: "user-a", ConversationID: "conv"}
	for i := 0; i < 3; i++ {
		if err := store.Append(context.Background(), key, schema.UserMessage("q"), schema.AssistantMessage("a", nil)); err != nil {
			t.Fatal(err)
		}
	}

	messages, version, err := store.Load(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || version != 3 {
		t.Fatalf("messages = %d, version = %d; want 4, 3", len(messages), version)
	}
	for i, message := range messages {
		if i%2 == 0 && message.Role != schema.User {
			t.Fatalf("message %d role = %v, want user", i, message.Role)
		}
		if i%2 == 1 && message.Role != schema.Assistant {
			t.Fatalf("message %d role = %v, want assistant", i, message.Role)
		}
	}
}

func TestMemoryStoreAppendIfVersionPreventsOverwrite(t *testing.T) {
	store := NewMemoryStore(6)
	key := SessionKey{TenantID: "tenant-a", UserID: "user-a", ConversationID: "conv"}
	committed, err := store.AppendIfVersion(context.Background(), key, 0, schema.UserMessage("q1"), schema.AssistantMessage("a1", nil))
	if err != nil || !committed {
		t.Fatalf("first append committed=%v, err=%v", committed, err)
	}
	committed, err = store.AppendIfVersion(context.Background(), key, 0, schema.UserMessage("q2"), schema.AssistantMessage("a2", nil))
	if err != nil {
		t.Fatal(err)
	}
	if committed {
		t.Fatal("stale version overwrote conversation")
	}
	messages, version, err := store.Load(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 || len(messages) != 2 || messages[0].Content != "q1" {
		t.Fatalf("unexpected conversation after conflict: version=%d messages=%#v", version, messages)
	}
}

func TestCoordinatorSerializesSameSessionButAllowsDifferentSessions(t *testing.T) {
	coordinator := NewCoordinator()
	key := SessionKey{TenantID: "tenant-a", UserID: "user-a", ConversationID: "same"}
	otherKey := SessionKey{TenantID: "tenant-a", UserID: "user-a", ConversationID: "other"}

	started := make(chan string, 2)
	release := make(chan struct{})
	firstDone := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := coordinator.Run(context.Background(), key, func(context.Context) error {
			started <- "first"
			<-release
			close(firstDone)
			return nil
		}); err != nil {
			t.Errorf("first run: %v", err)
		}
	}()
	select {
	case got := <-started:
		if got != "first" {
			t.Fatalf("first callback = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("first callback did not start")
	}

	secondStarted := make(chan struct{})
	go func() {
		defer wg.Done()
		if err := coordinator.Run(context.Background(), key, func(context.Context) error {
			close(secondStarted)
			return nil
		}); err != nil {
			t.Errorf("second run: %v", err)
		}
	}()
	select {
	case <-secondStarted:
		t.Fatal("same session ran concurrently")
	case <-time.After(30 * time.Millisecond):
	}

	otherStarted := make(chan struct{})
	if err := coordinator.Run(context.Background(), otherKey, func(context.Context) error {
		close(otherStarted)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-otherStarted:
	case <-time.After(time.Second):
		t.Fatal("different session was blocked")
	}

	close(release)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first session did not finish")
	}
	wg.Wait()
}
