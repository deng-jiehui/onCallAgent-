package tools

import (
	"context"
	"io"
	"time"

	e_mcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

/*
	GetLogMcpTool

https://cloud.tencent.com/developer/mcp/server/11710
https://cloud.tencent.com/document/product/614/118699#90415b66-8edb-43a9-ad5a-c2b0a97f5eaf

https://www.cloudwego.io/zh/docs/eino/ecosystem_integration/tool/tool_mcp/
https://mcp-go.dev/clients
*/
func GetLogMcpTool(ctx context.Context) ([]tool.BaseTool, error) {
	tools, _, err := GetLogMcpToolWithCloser(ctx)
	return tools, err
}

func GetLogMcpToolWithCloser(ctx context.Context) ([]tool.BaseTool, io.Closer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// https://mcp-api.tencent-cloud.com/sse/XXXX
	mcpUrl, err := g.Cfg().Get(ctx, "mcp_url")
	if err != nil {
		return nil, nil, err
	}
	cli, err := client.NewSSEMCPClient(mcpUrl.String())
	if err != nil {
		return []tool.BaseTool{}, nil, err
	}
	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = cli.Start(initCtx)
	if err != nil {
		_ = cli.Close()
		return []tool.BaseTool{}, nil, err
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "example-client",
		Version: "1.0.0",
	}
	if _, err = cli.Initialize(initCtx, initRequest); err != nil {
		_ = cli.Close()
		return []tool.BaseTool{}, nil, err
	}
	mcpTools, err := e_mcp.GetTools(initCtx, &e_mcp.Config{Cli: cli})
	if err != nil {
		_ = cli.Close()
		return []tool.BaseTool{}, nil, err
	}
	return mcpTools, cli, nil
}
