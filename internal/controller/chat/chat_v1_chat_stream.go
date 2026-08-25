package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/conversation"
	"SuperBizAgent/internal/observability"
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) ChatStream(ctx context.Context, req *v1.ChatStreamReq) (res *v1.ChatStreamRes, err error) {
	ctx, finish := observability.StartRequest(ctx, "/api/chat_stream", "stream", req.Id)
	defer func() { finish(err) }()
	id := req.Id
	msg := req.Question
	key, err := sessionKey(ctx, id)
	if err != nil {
		return nil, err
	}

	client, err := c.service.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		return nil, err
	}

	session, err := c.turnSession(ctx, key)
	if err != nil {
		client.SendToClient("error", err.Error())
		return nil, err
	}
	var fullResponse strings.Builder
	done := make(chan error, 1)
	emit := func(event conversation.TurnEvent) error {
		if event.Message != nil {
			fullResponse.WriteString(event.Message.Content)
			client.SendToClient("message", event.Message.Content)
		}
		if !event.Done {
			return nil
		}
		if event.Err != nil {
			done <- event.Err
			return nil
		}
		completeResponse := fullResponse.String()
		if completeResponse == "" {
			done <- errors.New("chat agent returned empty stream")
			return nil
		}
		if appendErr := c.conversations.Append(ctx, key, schema.UserMessage(msg), schema.AssistantMessage(completeResponse, nil)); appendErr != nil {
			done <- appendErr
			return nil
		}
		client.SendToClient("done", "Stream completed")
		done <- nil
		return nil
	}
	if err := session.PushBuild(ctx, func(runCtx context.Context) (*adk.AgentInput, error) {
		return c.buildTurnInput(runCtx, key, msg)
	}, emit); err != nil {
		client.SendToClient("error", err.Error())
		return nil, err
	}
	select {
	case err := <-done:
		if err != nil {
			client.SendToClient("error", err.Error())
			return nil, err
		}
		return &v1.ChatStreamRes{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
