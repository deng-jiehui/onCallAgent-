package conversation

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/schema"
)

var (
	ErrInvalidSessionKey = errors.New("tenant, user, and conversation IDs are required")
	ErrInvalidMessage    = errors.New("user and assistant messages are required")
	ErrVersionConflict   = errors.New("conversation version conflict")
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

type MemoryStore struct {
	maxMessages int
	entries     sync.Map // map[SessionKey]*memoryEntry
}

type memoryEntry struct {
	mu       sync.RWMutex
	messages []*schema.Message
	version  int64
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
