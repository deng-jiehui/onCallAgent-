package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/observability"
	"context"
	"errors"
	"io"
	"strings"

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
		sr, streamErr := runner.Stream(runCtx, userMessage)
		if streamErr != nil {
			return streamErr
		}
		defer sr.Close()

		var fullResponse strings.Builder
		for {
			chunk, recvErr := sr.Recv()
			if errors.Is(recvErr, io.EOF) {
				completeResponse := fullResponse.String()
				if completeResponse == "" {
					return errors.New("chat agent returned empty stream")
				}
				if appendErr := c.conversations.Append(runCtx, key, schema.UserMessage(msg), schema.AssistantMessage(completeResponse, nil)); appendErr != nil {
					return appendErr
				}
				client.SendToClient("done", "Stream completed")
				return nil
			}
			if recvErr != nil {
				return recvErr
			}
			fullResponse.WriteString(chunk.Content)
			client.SendToClient("message", chunk.Content)
		}
	})
	if err != nil {
		client.SendToClient("error", err.Error())
		return nil, err
	}
	return &v1.ChatStreamRes{}, nil
}
