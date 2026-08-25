package conversation

import (
	"context"
	"errors"
	"sync"

	"github.com/cloudwego/eino/adk"
)

var (
	ErrTurnLoopRegistryFull    = errors.New("conversation turn loop registry is full")
	ErrTurnLoopFactoryRequired = errors.New("conversation turn loop factory is required")
)

// TurnLoopRegistry owns the in-process mapping from authenticated session keys
// to Eino TurnLoop sessions. It is an instance-local cache; durable transcript
// and checkpoint storage are separate concerns.
type TurnLoopRegistry struct {
	mu          sync.Mutex
	maxSessions int
	sessions    map[SessionKey]*TurnLoopSession
}

func NewTurnLoopRegistry(maxSessions int) *TurnLoopRegistry {
	if maxSessions <= 0 {
		maxSessions = 128
	}
	return &TurnLoopRegistry{
		maxSessions: maxSessions,
		sessions:    make(map[SessionKey]*TurnLoopSession),
	}
}

func (r *TurnLoopRegistry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sessions)
}

func (r *TurnLoopRegistry) GetOrCreate(ctx context.Context, key SessionKey, factory func(context.Context) (adk.Agent, error)) (*TurnLoopSession, error) {
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if factory == nil {
		return nil, ErrTurnLoopFactoryRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if session, ok := r.sessions[key]; ok {
		if !session.IsStopped() {
			return session, nil
		}
		delete(r.sessions, key)
	}
	if len(r.sessions) >= r.maxSessions {
		return nil, ErrTurnLoopRegistryFull
	}
	agent, err := factory(ctx)
	if err != nil {
		return nil, err
	}
	session, err := NewTurnLoopSession(ctx, key, agent)
	if err != nil {
		return nil, err
	}
	r.sessions[key] = session
	return session, nil
}

func (r *TurnLoopRegistry) Remove(ctx context.Context, key SessionKey) error {
	r.mu.Lock()
	session, ok := r.sessions[key]
	if ok {
		delete(r.sessions, key)
	}
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return session.Stop(ctx)
}

func (r *TurnLoopRegistry) StopAll(ctx context.Context) error {
	r.mu.Lock()
	sessions := make([]*TurnLoopSession, 0, len(r.sessions))
	for key, session := range r.sessions {
		sessions = append(sessions, session)
		delete(r.sessions, key)
	}
	r.mu.Unlock()

	var firstErr error
	for _, session := range sessions {
		if err := session.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
