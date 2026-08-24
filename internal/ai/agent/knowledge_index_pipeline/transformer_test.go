package knowledge_index_pipeline

import (
	"context"
	"testing"

	authn "SuperBizAgent/internal/auth"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

type passthroughTransformer struct{}

func (passthroughTransformer) Transform(_ context.Context, src []*schema.Document, _ ...document.TransformerOption) ([]*schema.Document, error) {
	return src, nil
}

func TestTenantScopedTransformerAddsTenantMetadata(t *testing.T) {
	transformer := tenantScopedTransformer{inner: passthroughTransformer{}}
	ctx := authn.WithPrincipal(context.Background(), authn.Principal{TenantID: "tenant-acme"})
	docs, err := transformer.Transform(ctx, []*schema.Document{{MetaData: map[string]any{"source": "runbook.md"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := docs[0].MetaData["tenant_id"]; got != "tenant-acme" {
		t.Fatalf("tenant metadata = %#v", got)
	}
}

func TestTenantScopedTransformerRejectsMissingTenant(t *testing.T) {
	transformer := tenantScopedTransformer{inner: passthroughTransformer{}}
	if _, err := transformer.Transform(context.Background(), []*schema.Document{{}}); err == nil {
		t.Fatal("expected missing tenant scope to fail")
	}
}

func TestStableChunkID(t *testing.T) {
	first := stableChunkID("告警处理手册.md", 2)
	if first != stableChunkID("告警处理手册.md", 2) {
		t.Fatal("stable chunk ID changed for the same input")
	}
	if first == stableChunkID("告警处理手册.md", 3) {
		t.Fatal("stable chunk ID ignored the split index")
	}
	if len(first) != 64 {
		t.Fatalf("want SHA-256 hex ID, got %q", first)
	}
}

func TestStableChunkIDNormalizesPathSeparators(t *testing.T) {
	windows := stableChunkID(`runbooks\alerts.md`, 0)
	unix := stableChunkID("runbooks/alerts.md", 0)
	if windows != unix {
		t.Fatalf("path separators changed the ID: %q != %q", windows, unix)
	}
}
