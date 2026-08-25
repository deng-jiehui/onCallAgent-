package conversation

import (
	"context"
	"testing"
	"time"
)

func TestRedisConversationLockerRequiresClient(t *testing.T) {
	if _, err := NewRedisConversationLocker(nil, time.Second); err != ErrRedisClient {
		t.Fatalf("nil redis client error=%v", err)
	}
}

func TestConversationLockKeyIncludesTenantUserConversation(t *testing.T) {
	key := SessionKey{TenantID: "tenant", UserID: "user", ConversationID: "conversation"}
	got := redisLockKey(key)
	for _, part := range []string{"tenant", "user", "conversation"} {
		if !containsString(got, part) {
			t.Fatalf("lock key %q missing %q", got, part)
		}
	}
	if _, err := (&RedisConversationLocker{}).Lock(context.Background(), SessionKey{}); err == nil {
		t.Fatal("invalid session key should be rejected")
	}
}

func containsString(value, part string) bool {
	return len(value) >= len(part) && (value == part || len(part) == 0 || indexString(value, part) >= 0)
}

func indexString(value, part string) int {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
