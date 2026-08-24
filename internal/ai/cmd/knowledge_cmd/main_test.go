package main

import "testing"

func TestSourceFilterExpressionNormalizesWindowsPath(t *testing.T) {
	got := sourceFilterExpression(`docs\runbooks\与下游对账发现差异.md`)
	want := `metadata["_source"] == "docs/runbooks/与下游对账发现差异.md"`
	if got != want {
		t.Fatalf("source filter = %q, want %q", got, want)
	}
}
