package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

var (
	ErrCacheMiss   = errors.New("conversation cache miss")
	ErrRedisClient = errors.New("redis client is required")
)

type ConversationCache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

type CachedStore struct {
	primary ConversationStore
	cache   ConversationCache
	ttl     time.Duration
}

func NewCachedStore(primary ConversationStore, cache ConversationCache, ttl time.Duration) *CachedStore {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &CachedStore{primary: primary, cache: cache, ttl: ttl}
}

func (s *CachedStore) Load(ctx context.Context, key SessionKey) ([]*schema.Message, int64, error) {
	if err := key.Validate(); err != nil {
		return nil, 0, err
	}
	cacheKey := key.StorageKey()
	if raw, err := s.cache.Get(ctx, cacheKey); err == nil {
		var snapshot cachedConversation
		if unmarshalErr := json.Unmarshal(raw, &snapshot); unmarshalErr == nil {
			return cloneMessages(snapshot.Messages), snapshot.Version, nil
		}
	} else if !errors.Is(err, ErrCacheMiss) {
		// Cache outages are deliberately treated as misses; PostgreSQL remains
		// the source of truth for reads.
	}
	messages, version, err := s.primary.Load(ctx, key)
	if err != nil {
		return nil, 0, err
	}
	if raw, marshalErr := json.Marshal(cachedConversation{Messages: messages, Version: version}); marshalErr == nil {
		// A cache write is best-effort after a successful source read. A Redis
		// outage must not make PostgreSQL-backed conversation reads unavailable.
		_ = s.cache.Set(ctx, cacheKey, raw, s.ttl)
	}
	return messages, version, nil
}

func (s *CachedStore) Append(ctx context.Context, key SessionKey, userMessage, assistantMessage *schema.Message) error {
	if err := s.primary.Append(ctx, key, userMessage, assistantMessage); err != nil {
		return err
	}
	s.invalidate(ctx, key)
	return nil
}

func (s *CachedStore) AppendIfVersion(ctx context.Context, key SessionKey, expectedVersion int64, userMessage, assistantMessage *schema.Message) (bool, error) {
	committed, err := s.primary.AppendIfVersion(ctx, key, expectedVersion, userMessage, assistantMessage)
	if err != nil || !committed {
		return committed, err
	}
	s.invalidate(ctx, key)
	return true, nil
}

func (s *CachedStore) AppendIdempotent(ctx context.Context, key SessionKey, idempotencyKey string, userMessage, assistantMessage *schema.Message) (bool, error) {
	store, ok := s.primary.(IdempotentConversationStore)
	if !ok {
		return false, errors.New("primary conversation store does not support idempotency keys")
	}
	committed, err := store.AppendIdempotent(ctx, key, idempotencyKey, userMessage, assistantMessage)
	if err != nil || !committed {
		return committed, err
	}
	s.invalidate(ctx, key)
	return true, nil
}

func (s *CachedStore) invalidate(ctx context.Context, key SessionKey) error {
	return s.cache.Delete(ctx, key.StorageKey())
}

func (s *CachedStore) Close() error {
	if closer, ok := s.cache.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

type cachedConversation struct {
	Messages []*schema.Message `json:"messages"`
	Version  int64             `json:"version"`
}

type RedisConversationCache struct {
	client redis.UniversalClient
}

func NewRedisConversationCache(client redis.UniversalClient) (*RedisConversationCache, error) {
	if client == nil {
		return nil, ErrRedisClient
	}
	return &RedisConversationCache{client: client}, nil
}

func (c *RedisConversationCache) Get(ctx context.Context, key string) ([]byte, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	return raw, err
}

func (c *RedisConversationCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisConversationCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c *RedisConversationCache) Close() error { return c.client.Close() }

var _ ConversationStore = (*CachedStore)(nil)
var _ IdempotentConversationStore = (*CachedStore)(nil)
