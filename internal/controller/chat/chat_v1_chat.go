package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/observability"
	"context"
	"errors"

	"github.com/cloudwego/eino/schema"
)

func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
	ctx, finish := observability.StartRequest(ctx, "/api/chat", "invoke", req.Id)
	defer func() { finish(err) }()
	id := req.Id
	msg := req.Question
	key, err := sessionKey(ctx, id)
	if err != nil {
		return nil, err
	}

	var answer string
	err = c.coordinator.Run(ctx, key, func(runCtx context.Context) error {
		history, _, loadErr := c.conversations.Load(runCtx, key)
		if loadErr != nil {
			return loadErr
		}
		userMessage := &chat_pipeline.UserMessage{
			ID:      id,
			Query:   msg,
			History: history,
		}
		runner, buildErr := chat_pipeline.BuildChatAgent(runCtx)
		if buildErr != nil {
			return buildErr
		}
		out, invokeErr := runner.Invoke(runCtx, userMessage)
		if invokeErr != nil {
			return invokeErr
		}
		if out == nil {
			return errors.New("chat agent returned empty response")
		}
		if appendErr := c.conversations.Append(runCtx, key, schema.UserMessage(msg), schema.AssistantMessage(out.Content, nil)); appendErr != nil {
			return appendErr
		}
		answer = out.Content
		return nil
	})
	if err != nil {
		return nil, err
	}
	res = &v1.ChatRes{
		Answer: answer,
	}

	return res, nil
}
