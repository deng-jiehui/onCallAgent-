package conversation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

var (
	ErrTurnLoopStopped = errors.New("conversation turn loop stopped")
	ErrTurnInput       = errors.New("conversation turn input is required")
)

// TurnEvent is the application-level event emitted for one queued turn.
type TurnEvent struct {
	Message *schema.Message
	Err     error
	Done    bool
}

type turnRequest struct {
	ctx   context.Context
	input *adk.AgentInput
	build func(context.Context) (*adk.AgentInput, error)
	emit  func(TurnEvent) error
}

// TurnLoopSession owns one Eino TurnLoop for one authenticated conversation.
// It does not contain transcript history or tenant policy; those remain in the
// conversation store and request context.
type TurnLoopSession struct {
	key  SessionKey
	loop *adk.TurnLoop[turnRequest, *schema.Message]

	stopOnce sync.Once
	stopped  atomic.Bool
	active   atomic.Int64
	lastUsed atomic.Int64
}

func NewTurnLoopSession(ctx context.Context, key SessionKey, agent adk.Agent) (*TurnLoopSession, error) {
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, errors.New("conversation turn loop agent is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	session := &TurnLoopSession{key: key}
	session.touch()
	session.loop = adk.NewTurnLoop(adk.TurnLoopConfig[turnRequest, *schema.Message]{
		GenInput: func(runCtx context.Context, _ *adk.TurnLoop[turnRequest, *schema.Message], items []turnRequest) (*adk.GenInputResult[turnRequest, *schema.Message], error) {
			if len(items) == 0 {
				return nil, ErrTurnInput
			}
			for index, item := range items {
				if item.ctx == nil {
					item.ctx = runCtx
				}
				if item.build != nil {
					input, err := item.build(item.ctx)
					if err != nil {
						if emitErr := item.emit(TurnEvent{Err: err, Done: true}); emitErr != nil {
							return nil, emitErr
						}
						continue
					}
					item.input = input
				}
				if item.input == nil || len(item.input.Messages) == 0 {
					if emitErr := item.emit(TurnEvent{Err: ErrTurnInput, Done: true}); emitErr != nil {
						return nil, emitErr
					}
					continue
				}
				remaining := append([]turnRequest{}, items[:index]...)
				remaining = append(remaining, items[index+1:]...)
				return &adk.GenInputResult[turnRequest, *schema.Message]{
					RunCtx:    item.ctx,
					Input:     cloneAgentInput(item.input),
					Consumed:  []turnRequest{item},
					Remaining: remaining,
				}, nil
			}
			return nil, ErrTurnInput
		},
		PrepareAgent: func(context.Context, *adk.TurnLoop[turnRequest, *schema.Message], []turnRequest) (adk.Agent, error) {
			return agent, nil
		},
		OnAgentEvents: session.handleAgentEvents,
	})
	session.loop.Run(ctx)
	return session, nil
}

func (s *TurnLoopSession) Key() SessionKey { return s.key }

func (s *TurnLoopSession) IsStopped() bool { return s.stopped.Load() }

func (s *TurnLoopSession) IsIdleSince(now time.Time, idle time.Duration) bool {
	if idle <= 0 || s.IsStopped() || s.active.Load() != 0 {
		return false
	}
	last := time.Unix(0, s.lastUsed.Load())
	return !last.IsZero() && now.Sub(last) >= idle
}

func (s *TurnLoopSession) touch() { s.lastUsed.Store(time.Now().UnixNano()) }

func (s *TurnLoopSession) Push(ctx context.Context, input *adk.AgentInput, emit func(TurnEvent) error) error {
	if input == nil {
		return ErrTurnInput
	}
	return s.push(ctx, cloneAgentInput(input), nil, emit)
}

// PushBuild queues a request whose input is constructed only when its turn
// starts. This is the safe path for loading conversation history.
func (s *TurnLoopSession) PushBuild(ctx context.Context, build func(context.Context) (*adk.AgentInput, error), emit func(TurnEvent) error) error {
	if build == nil {
		return ErrTurnInput
	}
	return s.push(ctx, nil, build, emit)
}

func (s *TurnLoopSession) push(ctx context.Context, input *adk.AgentInput, build func(context.Context) (*adk.AgentInput, error), emit func(TurnEvent) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if (input == nil || len(input.Messages) == 0) && build == nil || emit == nil {
		return ErrTurnInput
	}
	if err := contextErr(ctx); err != nil {
		return err
	}
	if s.IsStopped() {
		return ErrTurnLoopStopped
	}
	s.active.Add(1)
	s.touch()
	wrappedEmit := func(event TurnEvent) error {
		s.touch()
		err := emit(event)
		if event.Done || event.Err != nil || err != nil {
			s.active.Add(-1)
		}
		return err
	}
	accepted, _ := s.loop.Push(turnRequest{
		ctx:   ctx,
		input: input,
		build: build,
		emit:  wrappedEmit,
	})
	if !accepted {
		s.active.Add(-1)
		return ErrTurnLoopStopped
	}
	return nil
}

func (s *TurnLoopSession) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.stopOnce.Do(func() {
		s.stopped.Store(true)
		s.loop.Stop(adk.WithImmediate())
	})
	done := make(chan struct{})
	go func() {
		s.loop.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *TurnLoopSession) handleAgentEvents(_ context.Context, tc *adk.TurnContext[turnRequest, *schema.Message], events *adk.AsyncIterator[*adk.AgentEvent]) error {
	if tc == nil || len(tc.Consumed) != 1 {
		return fmt.Errorf("conversation turn expected one consumed request")
	}
	request := tc.Consumed[0]
	for {
		event, ok := events.Next()
		if !ok {
			return request.emit(TurnEvent{Done: true})
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			if err := request.emit(TurnEvent{Err: event.Err, Done: true}); err != nil {
				return err
			}
			return nil
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		output := event.Output.MessageOutput
		if output.IsStreaming {
			if output.MessageStream == nil {
				if err := request.emit(TurnEvent{Err: errors.New("streaming agent event has nil message stream"), Done: true}); err != nil {
					return err
				}
				return nil
			}
			for {
				chunk, recvErr := output.MessageStream.Recv()
				if errors.Is(recvErr, io.EOF) {
					break
				}
				if errors.Is(recvErr, context.Canceled) {
					return request.emit(TurnEvent{Err: recvErr, Done: true})
				}
				if recvErr != nil {
					return request.emit(TurnEvent{Err: recvErr, Done: true})
				}
				if err := request.emit(TurnEvent{Message: chunk}); err != nil {
					return err
				}
			}
		}
		if output.Message != nil {
			if err := request.emit(TurnEvent{Message: output.Message}); err != nil {
				return err
			}
		}
	}
}

func cloneAgentInput(input *adk.AgentInput) *adk.AgentInput {
	return &adk.AgentInput{
		Messages:        cloneMessages(input.Messages),
		EnableStreaming: input.EnableStreaming,
	}
}
