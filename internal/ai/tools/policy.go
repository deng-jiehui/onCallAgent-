package tools

import (
	"context"
	"errors"
	"fmt"

	authn "SuperBizAgent/internal/auth"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

var ErrToolDenied = errors.New("tool access denied")

type ToolPolicy struct {
	TenantIDs        []string
	Roles            []string
	RequirePrincipal bool
}

type ToolRegistry struct{}

func NewToolRegistry() *ToolRegistry { return &ToolRegistry{} }

func (r *ToolRegistry) Wrap(name string, base tool.BaseTool, policy ToolPolicy) (tool.BaseTool, error) {
	if base == nil {
		return nil, errors.New("tool is required")
	}
	invokable, ok := base.(tool.InvokableTool)
	if !ok {
		return nil, fmt.Errorf("tool %q is not invokable", name)
	}
	return &policyTool{name: name, delegate: invokable, policy: policy}, nil
}

type policyTool struct {
	name     string
	delegate tool.InvokableTool
	policy   ToolPolicy
}

func (t *policyTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.delegate.Info(ctx)
}

func (t *policyTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	if !toolAllowed(ctx, t.policy) {
		return "", fmt.Errorf("%w: %s", ErrToolDenied, t.name)
	}
	return t.delegate.InvokableRun(ctx, argumentsInJSON, opts...)
}

func toolAllowed(ctx context.Context, policy ToolPolicy) bool {
	principal, ok := authn.PrincipalFromContext(ctx)
	if !ok || principal.TenantID == "" || principal.UserID == "" {
		return false
	}
	if len(policy.TenantIDs) > 0 && !contains(policy.TenantIDs, principal.TenantID) {
		return false
	}
	if len(policy.Roles) == 0 {
		return true
	}
	for _, role := range principal.Roles {
		if contains(policy.Roles, role) {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

var _ tool.InvokableTool = (*policyTool)(nil)
