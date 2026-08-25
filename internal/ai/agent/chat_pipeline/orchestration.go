package chat_pipeline

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func BuildChatAgent(ctx context.Context) (r compose.Runnable[*UserMessage, *schema.Message], err error) {
	const (
		InputToRag      = "InputToRag"
		ChatTemplate    = "ChatTemplate"
		ReactAgent      = "ReactAgent"
		MilvusRetriever = "MilvusRetriever"
		InputToChat     = "InputToChat"
	)
	g := compose.NewGraph[*UserMessage, *schema.Message]()
	if err := addGraphStep("node", InputToRag, func() error {
		return g.AddLambdaNode(InputToRag, compose.InvokableLambdaWithOption(newInputToRagLambda), compose.WithNodeName("UserMessageToRag"))
	}); err != nil {
		return nil, err
	}
	chatTemplateKeyOfChatTemplate, err := newChatTemplate(ctx)
	if err != nil {
		return nil, err
	}
	if err := addGraphStep("node", ChatTemplate, func() error {
		return g.AddChatTemplateNode(ChatTemplate, chatTemplateKeyOfChatTemplate)
	}); err != nil {
		return nil, err
	}
	reactAgentKeyOfLambda, err := newReactAgentLambda(ctx)
	if err != nil {
		return nil, err
	}
	if err := addGraphStep("node", ReactAgent, func() error {
		return g.AddLambdaNode(ReactAgent, reactAgentKeyOfLambda, compose.WithNodeName("ReActAgent"))
	}); err != nil {
		return nil, err
	}
	milvusRetrieverKeyOfRetriever, err := newRetriever(ctx)
	if err != nil {
		return nil, err
	}
	// 注意下面的 output key 设置，把查询出来的设置为了documents，匹配 ChatTemplate 里面说prompt
	if err := addGraphStep("node", MilvusRetriever, func() error {
		return g.AddRetrieverNode(MilvusRetriever, milvusRetrieverKeyOfRetriever, compose.WithOutputKey("documents"))
	}); err != nil {
		return nil, err
	}
	if err := addGraphStep("node", InputToChat, func() error {
		return g.AddLambdaNode(InputToChat, compose.InvokableLambdaWithOption(newInputToChatLambda), compose.WithNodeName("UserMessageToChat"))
	}); err != nil {
		return nil, err
	}
	for _, edge := range [][2]string{
		{compose.START, InputToRag},
		{compose.START, InputToChat},
		{ReactAgent, compose.END},
		{InputToRag, MilvusRetriever},
		{MilvusRetriever, ChatTemplate},
		{InputToChat, ChatTemplate},
		{ChatTemplate, ReactAgent},
	} {
		if err := addGraphStep("edge", edge[0]+" -> "+edge[1], func() error {
			return g.AddEdge(edge[0], edge[1])
		}); err != nil {
			return nil, err
		}
	}
	r, err = g.Compile(ctx, compose.WithGraphName("ChatAgent"), compose.WithNodeTriggerMode(compose.AllPredecessor))
	if err != nil {
		return nil, fmt.Errorf("graph compile %q: %w", "ChatAgent", err)
	}
	return r, err
}

func addGraphStep(kind, name string, add func() error) error {
	if err := add(); err != nil {
		return fmt.Errorf("graph %s %q: %w", kind, name, err)
	}
	return nil
}
