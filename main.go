package main

import (
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	authn "SuperBizAgent/internal/auth"
	authcontroller "SuperBizAgent/internal/controller/auth"
	"SuperBizAgent/internal/controller/chat"
	"SuperBizAgent/internal/conversation"
	"SuperBizAgent/internal/observability"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/middleware"
	"context"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"
)

func main() {
	ctx := gctx.New()
	authConfig, err := authn.LoadConfig()
	if err != nil {
		g.Log().Errorf(ctx, "initialize local JWT authentication: %v", err)
		panic(err)
	}
	authService, err := authn.NewService(authConfig)
	if err != nil {
		g.Log().Errorf(ctx, "validate local JWT authentication: %v", err)
		panic(err)
	}
	agentRuntime, err := chat_pipeline.NewRuntime(ctx)
	if err != nil {
		g.Log().Errorf(ctx, "initialize chat agent runtime: %v", err)
		panic(err)
	}
	shutdown, err := observability.Init(ctx, observability.LoadConfig(ctx))
	if err != nil {
		g.Log().Errorf(ctx, "initialize observability: %v", err)
		shutdown, _ = observability.Init(ctx, observability.Config{ServiceName: "superbizagent", SampleRatio: 1})
	}
	callbacks.AppendGlobalHandlers(observability.EinoHandler(observability.Instruments()))
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			g.Log().Errorf(ctx, "shutdown observability: %v", err)
		}
	}()
	fileDir, err := g.Cfg().Get(ctx, "file_dir")
	if err != nil {
		panic(err)
	}
	common.FileDir = fileDir.String()
	storeConfig, err := conversation.LoadStoreConfig(ctx)
	if err != nil {
		g.Log().Errorf(ctx, "load conversation store config: %v", err)
		panic(err)
	}
	conversationStore, conversationDB, err := conversation.OpenConfiguredStore(ctx, storeConfig)
	if err != nil {
		g.Log().Errorf(ctx, "initialize conversation store: %v", err)
		panic(err)
	}
	if conversationDB != nil {
		defer func() {
			if err := conversationDB.Close(); err != nil {
				g.Log().Errorf(ctx, "close conversation database: %v", err)
			}
		}()
	}
	if storeCloser, ok := conversationStore.(interface{ Close() error }); ok {
		defer func() {
			if err := storeCloser.Close(); err != nil {
				g.Log().Errorf(ctx, "close conversation cache: %v", err)
			}
		}()
	}
	s := g.Server()
	chatController := chat.NewV1WithStore(agentRuntime, conversationStore)
	if closable, ok := chatController.(interface{ Close(context.Context) error }); ok {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := closable.Close(shutdownCtx); err != nil {
				g.Log().Errorf(ctx, "shutdown chat sessions: %v", err)
			}
		}()
	}
	s.Group("/api/auth", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(authcontroller.New(authService))
	})
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(authService.Middleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(chatController)
	})
	s.SetPort(6872)
	s.Run()
}
