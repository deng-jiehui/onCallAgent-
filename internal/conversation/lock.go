package conversation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrLockUnavailable = errors.New("conversation lock unavailable")
	ErrLockReleased    = errors.New("conversation lock already released")
)

type ConversationLease interface {
	Renew(ctx context.Context) error
	Unlock(ctx context.Context) error
	StartRenewal(ctx context.Context) <-chan error
}

type ConversationLocker interface {
	Lock(ctx context.Context, key SessionKey) (ConversationLease, error)
}

type RedisConversationLocker struct {
	client   redis.UniversalClient
	ttl      time.Duration
	interval time.Duration
}

func NewRedisConversationLocker(client redis.UniversalClient, ttl time.Duration) (*RedisConversationLocker, error) {
	if client == nil {
		return nil, ErrRedisClient
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	interval := ttl / 3
	if interval < time.Second {
		interval = time.Second
	}
	return &RedisConversationLocker{client: client, ttl: ttl, interval: interval}, nil
}

func (l *RedisConversationLocker) Lock(ctx context.Context, key SessionKey) (ConversationLease, error) {
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	token, err := randomLockToken()
	if err != nil {
		return nil, err
	}
	lockKey := redisLockKey(key)
	for {
		ok, setErr := l.client.SetNX(ctx, lockKey, token, l.ttl).Result()
		if setErr != nil {
			return nil, setErr
		}
		if ok {
			return &redisConversationLease{client: l.client, key: lockKey, token: token, ttl: l.ttl, interval: l.interval}, nil
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type redisConversationLease struct {
	client   redis.UniversalClient
	key      string
	token    string
	ttl      time.Duration
	interval time.Duration
}

func (l *redisConversationLease) Renew(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	result := l.client.Eval(ctx, `
		if redis.call('get', KEYS[1]) == ARGV[1] then
			return redis.call('pexpire', KEYS[1], ARGV[2])
		end
		return 0`, []string{l.key}, l.token, l.ttl.Milliseconds())
	value, err := result.Int()
	if err != nil {
		return err
	}
	if value != 1 {
		return ErrLockUnavailable
	}
	return nil
}

func (l *redisConversationLease) Unlock(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	value, err := l.client.Eval(ctx, `
		if redis.call('get', KEYS[1]) == ARGV[1] then
			return redis.call('del', KEYS[1])
		end
		return 0`, []string{l.key}, l.token).Int()
	if err != nil {
		return err
	}
	if value != 1 {
		return ErrLockReleased
	}
	return nil
}

func (l *redisConversationLease) StartRenewal(ctx context.Context) <-chan error {
	done := make(chan error, 1)
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(l.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := l.Renew(ctx); err != nil {
					done <- err
					return
				}
			}
		}
	}()
	return done
}

func redisLockKey(key SessionKey) string {
	return fmt.Sprintf("conversation:lock:%s", key.StorageKey())
}

func randomLockToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
