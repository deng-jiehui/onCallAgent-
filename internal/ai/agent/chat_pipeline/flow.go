package chat_pipeline

import (
	"SuperBizAgent/internal/ai/tools"
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
)

func newReactAgentLambda(ctx context.Context) (lba *compose.Lambda, err error) {
	config := &react.AgentConfig{
		MaxStep:            25,
		ToolReturnDirectly: map[string]struct{}{}}
	chatModelIns11, err := newChatModel(ctx)
	if err != nil {
		return nil, err
	}
	config.ToolCallingModel = chatModelIns11
	//searchTool, err := newSearchTool(ctx)
	//if err != nil {
	//	return nil, err
	//}
	mcpTool, err := tools.GetLogMcpTool(ctx)
	if err != nil {
		log.Printf("mcp tools unavailable, continuing without mcp tools: %v", err)
		mcpTool = nil
	}
	config.ToolsConfig.Tools = mcpTool
	config.ToolsConfig.Tools = append(config.ToolsConfig.Tools, tools.NewPrometheusAlertsQueryTool())
	mysqlTool, err := tools.NewMysqlCrudTool()
	if err != nil {
		return nil, fmt.Errorf("initialize mysql tool: %w", err)
	}
	config.ToolsConfig.Tools = append(config.ToolsConfig.Tools, mysqlTool)
	config.ToolsConfig.Tools = append(config.ToolsConfig.Tools, tools.NewGetCurrentTimeTool())
	config.ToolsConfig.Tools = append(config.ToolsConfig.Tools, tools.NewQueryInternalDocsTool())

	ins, err := react.NewAgent(ctx, config)
	if err != nil {
		return nil, err
	}
	lba, err = compose.AnyLambda(ins.Generate, ins.Stream, nil, nil)
	if err != nil {
		return nil, err
	}
	return lba, nil
}
