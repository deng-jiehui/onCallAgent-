package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"
)

var (
	ErrInvalidSessionKey   = errors.New("tenant, user, and conversation IDs are required")
	ErrInvalidMessage      = errors.New("user and assistant messages are required")
	ErrVersionConflict     = errors.New("conversation version conflict")
	ErrIdempotencyConflict = errors.New("conversation idempotency key was reused with different content")
)

type SessionKey struct {
	TenantID       string
	UserID         string
	ConversationID string
}

func (k SessionKey) Validate() error {
	if k.TenantID == "" || k.UserID == "" || k.ConversationID == "" {
		return ErrInvalidSessionKey
	}
	return nil
}

func (k SessionKey) StorageKey() string {
	return fmt.Sprintf("conversation:%s:%s:%s", k.TenantID, k.UserID, k.ConversationID)
}

type ConversationStore interface {
	Load(ctx context.Context, key SessionKey) ([]*schema.Message, int64, error)
	Append(ctx context.Context, key SessionKey, userMessage, assistantMessage *schema.Message) error
	AppendIfVersion(ctx context.Context, key SessionKey, expectedVersion int64, userMessage, assistantMessage *schema.Message) (bool, error)
}

// IdempotentConversationStore atomically records a turn and its retry key.
// committed=false means the same request key was already committed.
type IdempotentConversationStore interface {
	ConversationStore
	AppendIdempotent(ctx context.Context, key SessionKey, idempotencyKey string, userMessage, assistantMessage *schema.Message) (committed bool, err error)
}

type MemoryStore struct {
	maxMessages int
	entries     sync.Map // map[SessionKey]*memoryEntry
	idempotency sync.Map // map[SessionKey]*memoryIdempotency
}

type memoryEntry struct {
	mu       sync.RWMutex
	messages []*schema.Message
	version  int64
}

type memoryIdempotency struct {
	mu     sync.Mutex
	values map[string]string
}

func NewMemoryStore(maxMessages int) *MemoryStore {
	if maxMessages < 2 {
		maxMessages = 6
	}
	if maxMessages%2 != 0 {
		maxMessages--
	}
	return &MemoryStore{maxMessages: maxMessages}
}

func (s *MemoryStore) Load(ctx context.Context, key SessionKey) ([]*schema.Message, int64, error) {
	if err := contextErr(ctx); err != nil {
		return nil, 0, err
	}
	if err := key.Validate(); err != nil {
		return nil, 0, err
	}
	entry := s.entry(key)
	entry.mu.RLock()
	defer entry.mu.RUnlock()
	return cloneMessages(entry.messages), entry.version, nil
}

func (s *MemoryStore) Append(ctx context.Context, key SessionKey, userMessage, assistantMessage *schema.Message) error {
	_, err := s.append(ctx, key, 0, userMessage, assistantMessage, false)
	return err
}

func (s *MemoryStore) AppendIfVersion(ctx context.Context, key SessionKey, expectedVersion int64, userMessage, assistantMessage *schema.Message) (bool, error) {
	return s.append(ctx, key, expectedVersion, userMessage, assistantMessage, true)
}

func (s *MemoryStore) AppendIdempotent(ctx context.Context, key SessionKey, idempotencyKey string, userMessage, assistantMessage *schema.Message) (bool, error) {
	if err := contextErr(ctx); err != nil {
		return false, err
	}
	if err := key.Validate(); err != nil {
		return false, err
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return false, errors.New("idempotency key is required")
	}
	if userMessage == nil || assistantMessage == nil {
		return false, ErrInvalidMessage
	}
	hash, err := messagePairHash(userMessage, assistantMessage)
	if err != nil {
		return false, err
	}
	marker := s.idempotencyEntry(key)
	marker.mu.Lock()
	defer marker.mu.Unlock()
	if previous, ok := marker.values[idempotencyKey]; ok {
		if previous != hash {
			return false, ErrIdempotencyConflict
		}
		return false, nil
	}
	if err := s.Append(ctx, key, userMessage, assistantMessage); err != nil {
		return false, err
	}
	marker.values[idempotencyKey] = hash
	return true, nil
}

func (s *MemoryStore) idempotencyEntry(key SessionKey) *memoryIdempotency {
	if entry, ok := s.idempotency.Load(key); ok {
		return entry.(*memoryIdempotency)
	}
	entry := &memoryIdempotency{values: make(map[string]string)}
	actual, _ := s.idempotency.LoadOrStore(key, entry)
	return actual.(*memoryIdempotency)
}

func messagePairHash(userMessage, assistantMessage *schema.Message) (string, error) {
	payload, err := json.Marshal([]*schema.Message{userMessage, assistantMessage})
	if err != nil {
		return "", fmt.Errorf("encode idempotency payload: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (s *MemoryStore) append(ctx context.Context, key SessionKey, expectedVersion int64, userMessage, assistantMessage *schema.Message, checkVersion bool) (bool, error) {
	if err := contextErr(ctx); err != nil {
		return false, err
	}
	if err := key.Validate(); err != nil {
		return false, err
	}
	if userMessage == nil || assistantMessage == nil {
		return false, ErrInvalidMessage
	}
	entry := s.entry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if checkVersion && entry.version != expectedVersion {
		return false, nil
	}
	entry.messages = append(entry.messages, cloneMessage(userMessage), cloneMessage(assistantMessage))
	if len(entry.messages) > s.maxMessages {
		excess := len(entry.messages) - s.maxMessages
		if excess%2 != 0 {
			excess++
		}
		entry.messages = entry.messages[excess:]
	}
	entry.version++
	return true, nil
}

func (s *MemoryStore) entry(key SessionKey) *memoryEntry {
	if entry, ok := s.entries.Load(key); ok {
		return entry.(*memoryEntry)
	}
	entry := &memoryEntry{}
	actual, _ := s.entries.LoadOrStore(key, entry)
	return actual.(*memoryEntry)
}

type Coordinator struct {
	locks sync.Map // map[SessionKey]*sessionLock
}

type sessionLock struct {
	token chan struct{}
}

func NewCoordinator() *Coordinator {
	return &Coordinator{}
}

func (c *Coordinator) Run(ctx context.Context, key SessionKey, fn func(context.Context) error) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if err := key.Validate(); err != nil {
		return err
	}
	if fn == nil {
		return errors.New("conversation callback is required")
	}
	lock := c.lock(key)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-lock.token:
	}
	defer func() { lock.token <- struct{}{} }()
	return fn(ctx)
}

func (c *Coordinator) lock(key SessionKey) *sessionLock {
	if lock, ok := c.locks.Load(key); ok {
		return lock.(*sessionLock)
	}
	lock := &sessionLock{token: make(chan struct{}, 1)}
	lock.token <- struct{}{}
	actual, _ := c.locks.LoadOrStore(key, lock)
	return actual.(*sessionLock)
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]*schema.Message, len(messages))
	for i, message := range messages {
		cloned[i] = cloneMessage(message)
	}
	return cloned
}

func cloneMessage(message *schema.Message) *schema.Message {
	if message == nil {
		return nil
	}
	cloned := *message
	cloned.MultiContent = append([]schema.ChatMessagePart(nil), message.MultiContent...)
	cloned.UserInputMultiContent = append([]schema.MessageInputPart(nil), message.UserInputMultiContent...)
	cloned.AssistantGenMultiContent = append([]schema.MessageOutputPart(nil), message.AssistantGenMultiContent...)
	cloned.ToolCalls = append([]schema.ToolCall(nil), message.ToolCalls...)
	if message.Extra != nil {
		cloned.Extra = make(map[string]any, len(message.Extra))
		for key, value := range message.Extra {
			cloned.Extra[key] = value
		}
	}
	return &cloned
}
