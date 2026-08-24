package main

import "testing"

func TestSourceFilterExpressionNormalizesWindowsPath(t *testing.T) {
	got := sourceFilterExpression(`docs\runbooks\与下游对账发现差异.md`)
	want := `metadata["_source"] == "docs/runbooks/与下游对账发现差异.md"`
	if got != want {
		t.Fatalf("source filter = %q, want %q", got, want)
	}
}

func TestTenantSourceFilterExpressionScopesDeletion(t *testing.T) {
	got := tenantSourceFilterExpression("docs/runbooks/alerts.md", "tenant-acme")
	want := `metadata["_source"] == "docs/runbooks/alerts.md" and metadata["tenant_id"] == "tenant-acme"`
	if got != want {
		t.Fatalf("tenant source filter = %q, want %q", got, want)
	}
}
