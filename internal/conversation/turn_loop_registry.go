package conversation

import (
	"context"
	"errors"
	"sync"
	"time"

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
	idleTTL     time.Duration
	stopJanitor chan struct{}
	janitorDone chan struct{}
	stopOnce    sync.Once
}

func NewTurnLoopRegistry(maxSessions int) *TurnLoopRegistry {
	return newTurnLoopRegistry(maxSessions, 0)
}

// NewTurnLoopRegistryWithIdle enables bounded idle-session cleanup. A zero or
// negative TTL disables the janitor while retaining the session limit.
func NewTurnLoopRegistryWithIdle(maxSessions int, idleTTL time.Duration) *TurnLoopRegistry {
	return newTurnLoopRegistry(maxSessions, idleTTL)
}

func newTurnLoopRegistry(maxSessions int, idleTTL time.Duration) *TurnLoopRegistry {
	if maxSessions <= 0 {
		maxSessions = 128
	}
	r := &TurnLoopRegistry{
		maxSessions: maxSessions,
		sessions:    make(map[SessionKey]*TurnLoopSession),
		idleTTL:     idleTTL,
	}
	if idleTTL > 0 {
		r.stopJanitor = make(chan struct{})
		r.janitorDone = make(chan struct{})
		go r.runJanitor()
	}
	return r
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
	r.stopOnce.Do(func() {
		if r.stopJanitor != nil {
			close(r.stopJanitor)
			<-r.janitorDone
		}
	})
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

func (r *TurnLoopRegistry) runJanitor() {
	defer close(r.janitorDone)
	interval := r.idleTTL / 2
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.evictIdle(time.Now())
		case <-r.stopJanitor:
			return
		}
	}
}

func (r *TurnLoopRegistry) evictIdle(now time.Time) {
	var idle []*TurnLoopSession
	r.mu.Lock()
	for key, session := range r.sessions {
		if session.IsIdleSince(now, r.idleTTL) {
			delete(r.sessions, key)
			idle = append(idle, session)
		}
	}
	r.mu.Unlock()
	for _, session := range idle {
		_ = session.Stop(context.Background())
	}
}
