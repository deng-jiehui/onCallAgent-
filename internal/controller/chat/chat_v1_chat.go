package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/conversation"
	"SuperBizAgent/internal/observability"
	"context"
	"errors"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
	ctx, finish := observability.StartRequest(ctx, "/api/chat", "invoke", req.Id)
	defer func() { finish(err) }()
	id := req.Id
	msg := req.Question
	idempotencyKey := req.IdempotencyKey
	key, err := sessionKey(ctx, id)
	if err != nil {
		return nil, err
	}
	turnCtx, releaseLock, err := c.lockTurn(ctx, key)
	if err != nil {
		return nil, err
	}
	defer releaseLock()

	session, err := c.turnSession(ctx, key)
	if err != nil {
		return nil, err
	}
	events := make(chan conversation.TurnEvent, 4)
	if c.limiter != nil {
		if err := c.limiter.Acquire(turnCtx); err != nil {
			return nil, err
		}
	}
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			if c.limiter != nil {
				c.limiter.Release()
			}
		})
	}
	var answer string
	emit := func(event conversation.TurnEvent) error {
		if event.Message != nil {
			answer = event.Message.Content
		}
		if event.Done && event.Err == nil {
			if answer == "" {
				event.Err = errors.New("chat agent returned empty response")
			} else if appendErr := appendTurn(turnCtx, c.conversations, key, idempotencyKey, schema.UserMessage(msg), schema.AssistantMessage(answer, nil)); appendErr != nil {
				event.Err = appendErr
			}
		}
		if event.Done || event.Err != nil {
			release()
		}
		events <- event
		return nil
	}
	if err := session.PushBuild(turnCtx, func(runCtx context.Context) (*adk.AgentInput, error) {
		return c.buildTurnInput(runCtx, key, msg)
	}, emit); err != nil {
		release()
		return nil, err
	}
	for {
		select {
		case event := <-events:
			if event.Err != nil {
				return nil, event.Err
			}
			if event.Done {
				return &v1.ChatRes{Answer: answer}, nil
			}
		case <-turnCtx.Done():
			return nil, turnCtx.Err()
		}
	}
}
