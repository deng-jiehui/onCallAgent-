package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

type fakeConversationCache struct {
	value   []byte
	gets    int
	sets    int
	deletes int
	err     error
}

func (f *fakeConversationCache) Get(context.Context, string) ([]byte, error) {
	f.gets++
	if f.err != nil {
		return nil, f.err
	}
	if f.value == nil {
		return nil, ErrCacheMiss
	}
	return append([]byte(nil), f.value...), nil
}
func (f *fakeConversationCache) Set(_ context.Context, _ string, value []byte, _ time.Duration) error {
	f.sets++
	if f.err != nil {
		return f.err
	}
	f.value = append([]byte(nil), value...)
	return nil
}
func (f *fakeConversationCache) Delete(context.Context, string) error {
	f.deletes++
	if f.err != nil {
		return f.err
	}
	f.value = nil
	return nil
}

func TestCachedStoreLoadsCacheThenFallsBackToPrimary(t *testing.T) {
	primary := NewMemoryStore(6)
	key := SessionKey{TenantID: "tenant", UserID: "user", ConversationID: "conversation"}
	if err := primary.Append(context.Background(), key, schema.UserMessage("q"), schema.AssistantMessage("a", nil)); err != nil {
		t.Fatal(err)
	}
	cache := &fakeConversationCache{}
	store := NewCachedStore(primary, cache, time.Minute)
	messages, version, err := store.Load(context.Background(), key)
	if err != nil || version != 1 || len(messages) != 2 {
		t.Fatalf("fallback load messages=%d version=%d err=%v", len(messages), version, err)
	}
	if cache.sets != 1 {
		t.Fatalf("fallback load cache sets=%d, want 1", cache.sets)
	}
	if _, _, err := store.Load(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if cache.gets != 2 {
		t.Fatalf("cache gets=%d, want 2", cache.gets)
	}
}

func TestCachedStoreInvalidatesOnlyAfterSuccessfulAppend(t *testing.T) {
	primary := NewMemoryStore(6)
	key := SessionKey{TenantID: "tenant", UserID: "user", ConversationID: "conversation"}
	cache := &fakeConversationCache{}
	store := NewCachedStore(primary, cache, time.Minute)
	if err := store.Append(context.Background(), key, schema.UserMessage("q"), schema.AssistantMessage("a", nil)); err != nil {
		t.Fatal(err)
	}
	if cache.deletes != 1 {
		t.Fatalf("successful append deletes=%d, want 1", cache.deletes)
	}
	cache.err = errors.New("cache unavailable")
	if err := store.Append(context.Background(), key, schema.UserMessage("q2"), schema.AssistantMessage("a2", nil)); err != nil {
		t.Fatalf("cache failure must not hide primary commit: %v", err)
	}
	_, version, _ := primary.Load(context.Background(), key)
	if version != 2 {
		t.Fatalf("primary version=%d, want committed version 2", version)
	}
}
