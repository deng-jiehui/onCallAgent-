package log_call_back

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/callbacks"
)

type LogCallbackConfig struct {
	Detail bool
	Debug  bool
}

func LogCallback(config *LogCallbackConfig) callbacks.Handler {
	if config == nil {
		config = &LogCallbackConfig{
			Detail: true,
		}
	}

	formatRunInfo := func(info *callbacks.RunInfo) string {
		if info == nil {
			return "unknown:unknown:unknown"
		}
		return fmt.Sprintf("%s:%s:%s", info.Component, info.Type, info.Name)
	}

	builder := callbacks.NewHandlerBuilder()
	builder.OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		fmt.Printf("[view start]:[%s]\n", formatRunInfo(info))
		if config.Detail {
			var b []byte
			if config.Debug {
				b, _ = json.MarshalIndent(input, "", "  ")
			} else {
				b, _ = json.Marshal(input)
			}
			fmt.Printf("%s\n", string(b))
		}
		return ctx
	})
	builder.OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		fmt.Printf("[view end]:[%s]\n", formatRunInfo(info))
		return ctx
	})
	return builder.Build()
}
