package tools

import (
	"context"
	"errors"
	"testing"

	authn "SuperBizAgent/internal/auth"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

func TestToolRegistryDeniesMissingOrWrongPrincipal(t *testing.T) {
	registry := NewToolRegistry()
	wrapped, err := registry.Wrap("restricted", &policyTestTool{}, ToolPolicy{Roles: []string{"operator"}})
	if err != nil {
		t.Fatal(err)
	}
	invokable := wrapped.(tool.InvokableTool)
	if _, err := invokable.InvokableRun(context.Background(), `{}`); !errors.Is(err, ErrToolDenied) {
		t.Fatalf("missing principal error=%v", err)
	}
	ctx := authn.WithPrincipal(context.Background(), authn.Principal{TenantID: "tenant", UserID: "user", Roles: []string{"viewer"}})
	if _, err := invokable.InvokableRun(ctx, `{}`); !errors.Is(err, ErrToolDenied) {
		t.Fatalf("wrong role error=%v", err)
	}
}

func TestToolRegistryAllowsConfiguredTenantAndRole(t *testing.T) {
	registry := NewToolRegistry()
	wrapped, err := registry.Wrap("restricted", &policyTestTool{}, ToolPolicy{TenantIDs: []string{"tenant-a"}, Roles: []string{"operator"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := authn.WithPrincipal(context.Background(), authn.Principal{TenantID: "tenant-a", UserID: "user", Roles: []string{"operator"}})
	result, err := wrapped.(tool.InvokableTool).InvokableRun(ctx, `{}`)
	if err != nil || result != "ok" {
		t.Fatalf("allowed tool result=%q err=%v", result, err)
	}
}

type policyTestTool struct{}

func (*policyTestTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "restricted"}, nil
}
func (*policyTestTool) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	return "ok", nil
}
