package knowledge_index_pipeline

import (
	authn "SuperBizAgent/internal/auth"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

// newDocumentTransformer component initialization function of node 'MarkdownSplitter' in graph 'KnowledgeIndexing'
func newDocumentTransformer(ctx context.Context) (tfr document.Transformer, err error) {
	config := &markdown.HeaderConfig{
		Headers: map[string]string{
			"#": "title",
		},
		TrimHeaders: false,
		IDGenerator: func(ctx context.Context, originalID string, splitIndex int) string {
			return stableChunkID(originalID, splitIndex)
		},
	}
	inner, err := markdown.NewHeaderSplitter(ctx, config)
	if err != nil {
		return nil, err
	}
	return tenantScopedTransformer{inner: inner}, nil
}

type tenantScopedTransformer struct {
	inner document.Transformer
}

func (t tenantScopedTransformer) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	principal, ok := authn.PrincipalFromContext(ctx)
	if !ok || principal.TenantID == "" {
		return nil, errors.New("authenticated tenant scope is required for indexing")
	}
	docs, err := t.inner.Transform(ctx, src, opts...)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		if doc.MetaData == nil {
			doc.MetaData = make(map[string]any)
		}
		doc.MetaData["tenant_id"] = principal.TenantID
	}
	return docs, nil
}

func stableChunkID(originalID string, splitIndex int) string {
	normalizedID := filepath.ToSlash(originalID)
	// filepath.ToSlash only replaces the platform separator. Normalize Windows
	// separators as well so datasets remain stable across operating systems.
	for i := range normalizedID {
		if normalizedID[i] == '\\' {
			normalizedID = normalizedID[:i] + "/" + normalizedID[i+1:]
		}
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", normalizedID, splitIndex)))
	return hex.EncodeToString(sum[:])
}
