package conversation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/redis/go-redis/v9"
)

var (
	ErrConversationBackend = errors.New("conversation backend must be memory or postgres")
	ErrPostgresDSNRequired = errors.New("postgres conversation backend requires a DSN")
)

type StoreConfig struct {
	Backend     string
	DSN         string
	MaxMessages int
	RedisURL    string
	CacheTTL    time.Duration
}

func ParseStoreConfig(values map[string]string) (StoreConfig, error) {
	cfg := StoreConfig{Backend: "memory", MaxMessages: 20, CacheTTL: 5 * time.Minute}
	if value := strings.TrimSpace(values["backend"]); value != "" {
		cfg.Backend = strings.ToLower(value)
	}
	if cfg.Backend != "memory" && cfg.Backend != "postgres" {
		return StoreConfig{}, fmt.Errorf("%w: %s", ErrConversationBackend, cfg.Backend)
	}
	cfg.DSN = strings.TrimSpace(values["dsn"])
	cfg.RedisURL = strings.TrimSpace(values["redis_url"])
	if cfg.Backend == "postgres" && cfg.DSN == "" {
		return StoreConfig{}, ErrPostgresDSNRequired
	}
	if raw := strings.TrimSpace(values["max_messages"]); raw != "" {
		maxMessages, err := strconv.Atoi(raw)
		if err != nil || maxMessages < 2 {
			return StoreConfig{}, fmt.Errorf("invalid conversation max_messages: %q", raw)
		}
		cfg.MaxMessages = maxMessages
	}
	if cfg.MaxMessages%2 != 0 {
		cfg.MaxMessages--
	}
	if raw := strings.TrimSpace(values["cache_ttl"]); raw != "" {
		cacheTTL, err := time.ParseDuration(raw)
		if err != nil || cacheTTL <= 0 {
			return StoreConfig{}, fmt.Errorf("invalid conversation cache_ttl: %q", raw)
		}
		cfg.CacheTTL = cacheTTL
	}
	return cfg, nil
}

func LoadStoreConfig(ctx context.Context) (StoreConfig, error) {
	values := make(map[string]string, 3)
	for key, path := range map[string]string{
		"backend":      "conversation.backend",
		"dsn":          "conversation.postgres.dsn",
		"max_messages": "conversation.max_messages",
		"redis_url":    "conversation.redis.url",
		"cache_ttl":    "conversation.redis.cache_ttl",
	} {
		value, err := g.Cfg().Get(ctx, path)
		if err == nil && !value.IsNil() {
			values[key] = value.String()
		}
	}
	return ParseStoreConfig(values)
}

func OpenConfiguredStore(ctx context.Context, cfg StoreConfig) (ConversationStore, *sql.DB, error) {
	if cfg.Backend == "memory" {
		return NewMemoryStore(cfg.MaxMessages), nil, nil
	}
	if cfg.Backend != "postgres" {
		return nil, nil, fmt.Errorf("%w: %s", ErrConversationBackend, cfg.Backend)
	}
	db, err := openPostgres(ctx, cfg.DSN)
	if err != nil {
		return nil, nil, err
	}
	store, err := NewSQLStore(db, cfg.MaxMessages)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	if cfg.RedisURL == "" {
		return store, db, nil
	}
	options, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("parse conversation redis URL: %w", err)
	}
	redisClient := redis.NewClient(options)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		return nil, nil, fmt.Errorf("ping conversation redis: %w", err)
	}
	cache, err := NewRedisConversationCache(redisClient)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		return nil, nil, err
	}
	return NewCachedStore(store, cache, cfg.CacheTTL), db, nil
}
