package knowledge_index_pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino/components/document"
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
	tfr, err = markdown.NewHeaderSplitter(ctx, config)
	if err != nil {
		return nil, err
	}
	return tfr, nil
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
