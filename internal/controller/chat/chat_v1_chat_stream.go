package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/conversation"
	"SuperBizAgent/internal/logic/sse"
	"SuperBizAgent/internal/observability"
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) ChatStream(ctx context.Context, req *v1.ChatStreamReq) (res *v1.ChatStreamRes, err error) {
	ctx, finish := observability.StartRequest(ctx, "/api/chat_stream", "stream", req.Id)
	defer func() { finish(err) }()
	id := req.Id
	msg := req.Question
	idempotencyKey := req.IdempotencyKey
	key, err := sessionKey(ctx, id)
	if err != nil {
		return nil, err
	}

	client, err := c.service.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		return nil, err
	}
	turnCtx, releaseLock, err := c.lockTurn(ctx, key)
	if err != nil {
		_ = client.SendToClient("error", err.Error())
		client.Finish()
		return nil, err
	}
	defer releaseLock()

	session, err := c.turnSession(ctx, key)
	if err != nil {
		client.SendToClient("error", err.Error())
		return nil, err
	}
	var fullResponse strings.Builder
	done := make(chan error, 1)
	if c.limiter != nil {
		if err := c.limiter.Acquire(turnCtx); err != nil {
			_ = client.SendToClient("error", err.Error())
			client.Finish()
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
	finishClient := func() {
		client.Finish()
	}
	send := func(eventType, data string) error {
		if client.SendToClient(eventType, data) {
			return nil
		}
		return sse.ErrUnavailable
	}
	emit := func(event conversation.TurnEvent) error {
		if event.Message != nil {
			fullResponse.WriteString(event.Message.Content)
			if sendErr := send("message", event.Message.Content); sendErr != nil {
				release()
				done <- sendErr
				return sendErr
			}
		}
		if !event.Done {
			return nil
		}
		if event.Err != nil {
			release()
			done <- event.Err
			return nil
		}
		completeResponse := fullResponse.String()
		if completeResponse == "" {
			release()
			done <- errors.New("chat agent returned empty stream")
			return nil
		}
		if appendErr := appendTurn(turnCtx, c.conversations, key, idempotencyKey, schema.UserMessage(msg), schema.AssistantMessage(completeResponse, nil)); appendErr != nil {
			release()
			done <- appendErr
			return nil
		}
		if sendErr := send("done", "Stream completed"); sendErr != nil {
			release()
			done <- sendErr
			return sendErr
		}
		done <- nil
		release()
		return nil
	}
	if err := session.PushBuild(turnCtx, func(runCtx context.Context) (*adk.AgentInput, error) {
		return c.buildTurnInput(runCtx, key, msg)
	}, emit); err != nil {
		release()
		_ = send("error", err.Error())
		finishClient()
		return nil, err
	}
	select {
	case err := <-done:
		if err != nil {
			_ = send("error", err.Error())
			finishClient()
			return nil, err
		}
		finishClient()
		return &v1.ChatStreamRes{}, nil
	case <-turnCtx.Done():
		finishClient()
		return nil, turnCtx.Err()
	}
}
