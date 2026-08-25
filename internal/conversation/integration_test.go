//go:build integration

package conversation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

func TestPostgresConversationStoreIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := openPostgres(ctx, integrationPostgresDSN())
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer db.Close()
	store, err := NewSQLStore(db, 10)
	if err != nil {
		t.Fatal(err)
	}
	key := integrationKey("postgres")
	otherTenant := key
	otherTenant.TenantID = key.TenantID + "-other"
	user := schema.UserMessage("question")
	assistant := schema.AssistantMessage("answer", nil)
	if err := store.Append(ctx, key, user, assistant); err != nil {
		t.Fatalf("append: %v", err)
	}
	messages, version, err := store.Load(ctx, key)
	if err != nil || version != 1 || len(messages) != 2 {
		t.Fatalf("load messages=%d version=%d err=%v", len(messages), version, err)
	}
	otherMessages, _, err := store.Load(ctx, otherTenant)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherMessages) != 0 {
		t.Fatal("other tenant saw conversation messages")
	}
	committed, err := store.AppendIfVersion(ctx, key, 0, schema.UserMessage("stale"), schema.AssistantMessage("stale", nil))
	if err != nil || committed {
		t.Fatalf("stale append committed=%v err=%v", committed, err)
	}
	committed, err = store.AppendIdempotent(ctx, key, "integration-idempotency", schema.UserMessage("q2"), schema.AssistantMessage("a2", nil))
	if err != nil || !committed {
		t.Fatalf("idempotent append committed=%v err=%v", committed, err)
	}
	committed, err = store.AppendIdempotent(ctx, key, "integration-idempotency", schema.UserMessage("q2"), schema.AssistantMessage("a2", nil))
	if err != nil || committed {
		t.Fatalf("duplicate append committed=%v err=%v", committed, err)
	}
}

func TestRedisCacheAndConversationLockIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	redisClient := redis.NewClient(&redis.Options{Addr: integrationRedisAddr()})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	defer redisClient.Close()
	db, err := openPostgres(ctx, integrationPostgresDSN())
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer db.Close()
	resources, err := OpenConfiguredStoreResources(ctx, StoreConfig{
		Backend: "postgres", DSN: integrationPostgresDSN(),
		RedisURL: "redis://localhost:56379/14", CacheTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("configured store resources: %v", err)
	}
	defer resources.DB.Close()
	if resources.Locker == nil {
		t.Fatal("configured resources did not create redis locker")
	}
	if closer, ok := resources.Store.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	primary, err := NewSQLStore(db, 10)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := NewRedisConversationCache(redisClient)
	if err != nil {
		t.Fatal(err)
	}
	store := NewCachedStore(primary, cache, time.Minute)
	key := integrationKey("cache")
	if err := store.Append(ctx, key, schema.UserMessage("cached question"), schema.AssistantMessage("cached answer", nil)); err != nil {
		t.Fatalf("cached append: %v", err)
	}
	if _, err := redisClient.Exists(ctx, key.StorageKey()).Result(); err != nil {
		t.Fatalf("cache key lookup: %v", err)
	}
	if _, _, err := store.Load(ctx, key); err != nil {
		t.Fatalf("cached load: %v", err)
	}
	cacheExists, err := redisClient.Exists(ctx, key.StorageKey()).Result()
	if err != nil || cacheExists != 1 {
		t.Fatalf("expected cached snapshot, exists=%d err=%v", cacheExists, err)
	}
	if err := store.Append(ctx, key, schema.UserMessage("next question"), schema.AssistantMessage("next answer", nil)); err != nil {
		t.Fatalf("append after cache fill: %v", err)
	}
	cacheExists, err = redisClient.Exists(ctx, key.StorageKey()).Result()
	if err != nil || cacheExists != 0 {
		t.Fatalf("expected cache invalidation, exists=%d err=%v", cacheExists, err)
	}

	locker, err := NewRedisConversationLocker(redisClient, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	first, err := locker.Lock(ctx, key)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	secondCtx, secondCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer secondCancel()
	if _, err := locker.Lock(secondCtx, key); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock error=%v, want deadline exceeded", err)
	}
	if err := first.Renew(ctx); err != nil {
		t.Fatalf("renew lock: %v", err)
	}
	if err := first.Unlock(ctx); err != nil {
		t.Fatalf("unlock first lock: %v", err)
	}
	second, err := locker.Lock(ctx, key)
	if err != nil {
		t.Fatalf("second lock after release: %v", err)
	}
	if err := second.Unlock(ctx); err != nil {
		t.Fatalf("unlock second lock: %v", err)
	}
}

func integrationPostgresDSN() string {
	if value := os.Getenv("SUPERBIZ_IT_POSTGRES_DSN"); value != "" {
		return value
	}
	return "postgres://superbizagent:superbizagent@localhost:55432/superbizagent?sslmode=disable"
}

func integrationRedisAddr() string {
	if value := os.Getenv("SUPERBIZ_IT_REDIS_ADDR"); value != "" {
		return value
	}
	return "localhost:56379"
}

func integrationKey(suffix string) SessionKey {
	return SessionKey{
		TenantID:       fmt.Sprintf("integration-tenant-%s-%d", suffix, time.Now().UnixNano()),
		UserID:         "integration-user",
		ConversationID: "integration-conversation",
	}
}
