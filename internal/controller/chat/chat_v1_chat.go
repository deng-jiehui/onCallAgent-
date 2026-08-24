package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/observability"
	"SuperBizAgent/utility/mem"
	"context"

	"github.com/cloudwego/eino/schema"
)

func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
	ctx, finish := observability.StartRequest(ctx, "/api/chat", "invoke", req.Id)
	defer func() { finish(err) }()
	id := req.Id
	msg := req.Question
	userMessage := &chat_pipeline.UserMessage{
		ID:      id,
		Query:   msg,
		History: mem.GetSimpleMemory(id).GetMessages(),
	}

	runner, err := chat_pipeline.BuildChatAgent(ctx)
	if err != nil {
		return nil, err
	}

	out, err := runner.Invoke(ctx, userMessage)
	if err != nil {
		return nil, err
	}
	res = &v1.ChatRes{
		Answer: out.Content,
	}
	mem.GetSimpleMemory(id).SetMessages(schema.UserMessage(msg))
	mem.GetSimpleMemory(id).SetMessages(schema.SystemMessage(out.Content))

	return res, nil
}
