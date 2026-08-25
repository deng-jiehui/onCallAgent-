package chat_pipeline

import (
	retriever2 "SuperBizAgent/internal/ai/retriever"
	"context"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

func newRetriever(ctx context.Context) (rtr retriever.Retriever, err error) {
	base, err := retriever2.NewMilvusRetriever(ctx)
	if err != nil {
		return nil, err
	}
	return &intentAwareRetriever{delegate: base}, nil
}

type intentAwareRetriever struct {
	delegate retriever.Retriever
}

func (r *intentAwareRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	if !shouldRetrieve(query) {
		return []*schema.Document{}, nil
	}
	return r.delegate.Retrieve(ctx, query, opts...)
}
