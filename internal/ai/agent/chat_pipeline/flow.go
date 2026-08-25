package chat_pipeline

import (
	"SuperBizAgent/internal/ai/tools"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
)

func newReactAgentLambda(ctx context.Context) (lba *compose.Lambda, closer io.Closer, err error) {
	config := &react.AgentConfig{
		MaxStep:            25,
		ToolReturnDirectly: map[string]struct{}{}}
	chatModelIns11, err := newChatModel(ctx)
	if err != nil {
		return nil, nil, err
	}
	config.ToolCallingModel = chatModelIns11
	registry := tools.NewToolRegistry()
	appendPolicyTool := func(name string, base tool.BaseTool, policy tools.ToolPolicy) error {
		wrapped, wrapErr := registry.Wrap(name, base, policy)
		if wrapErr != nil {
			return wrapErr
		}
		config.ToolsConfig.Tools = append(config.ToolsConfig.Tools, wrapped)
		return nil
	}
	//searchTool, err := newSearchTool(ctx)
	//if err != nil {
	//	return nil, err
	//}
	mcpTool, mcpCloser, err := tools.GetLogMcpToolWithCloser(ctx)
	if err != nil {
		log.Printf("mcp tools unavailable, continuing without mcp tools: %v", err)
		mcpTool = nil
	}
	closer = mcpCloser
	cleanupOnError := true
	defer func() {
		if cleanupOnError && closer != nil {
			_ = closer.Close()
		}
	}()
	for _, mcp := range mcpTool {
		info, infoErr := mcp.Info(ctx)
		if infoErr != nil {
			return nil, nil, fmt.Errorf("inspect mcp tool: %w", infoErr)
		}
		if err := appendPolicyTool(info.Name, mcp, tools.ToolPolicy{Roles: []string{"operator", "admin"}}); err != nil {
			return nil, nil, err
		}
	}
	if err := appendPolicyTool("prometheus_alerts", tools.NewPrometheusAlertsQueryTool(), tools.ToolPolicy{Roles: []string{"operator", "admin"}}); err != nil {
		return nil, nil, err
	}
	mysqlTool, err := tools.NewMysqlCrudTool()
	if err != nil {
		return nil, nil, fmt.Errorf("initialize mysql tool: %w", err)
	}
	if err := appendPolicyTool("mysql_crud", mysqlTool, tools.ToolPolicy{Roles: []string{"operator", "admin"}}); err != nil {
		return nil, nil, err
	}
	if err := appendPolicyTool("get_current_time", tools.NewGetCurrentTimeTool(), tools.ToolPolicy{}); err != nil {
		return nil, nil, err
	}
	if err := appendPolicyTool("query_internal_docs", tools.NewQueryInternalDocsTool(), tools.ToolPolicy{}); err != nil {
		return nil, nil, err
	}

	ins, err := react.NewAgent(ctx, config)
	if err != nil {
		return nil, nil, err
	}
	lba, err = compose.AnyLambda(ins.Generate, ins.Stream, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	cleanupOnError = false
	return lba, closer, nil
}
