package retriever

import (
	"SuperBizAgent/internal/ai/embedder"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/cloudwego/eino/components/embedding"
	einoRetriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	milvusClient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

type milvusRetriever struct {
	client    milvusClient.Client
	embedder  embedding.Embedder
	partition []string
	topK      int
}

func NewMilvusRetriever(ctx context.Context) (einoRetriever.Retriever, error) {
	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return nil, err
	}
	eb, err := embedder.DoubaoEmbedding(ctx)
	if err != nil {
		cli.Close()
		return nil, err
	}

	return &milvusRetriever{
		client:   cli,
		embedder: eb,
		topK:     1,
	}, nil
}

func (r *milvusRetriever) Retrieve(ctx context.Context, query string, opts ...einoRetriever.Option) ([]*schema.Document, error) {
	options := einoRetriever.GetCommonOptions(&einoRetriever.Options{
		TopK:      &r.topK,
		Embedding: r.embedder,
	}, opts...)

	topK := r.topK
	if options.TopK != nil {
		topK = *options.TopK
	}
	eb := r.embedder
	if options.Embedding != nil {
		eb = options.Embedding
	}
	if eb == nil {
		return nil, fmt.Errorf("embedding not provided")
	}

	vectors, err := eb.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("invalid embedding result length: got %d", len(vectors))
	}

	searchParam, err := entity.NewIndexAUTOINDEXSearchParam(1)
	if err != nil {
		return nil, err
	}
	results, err := r.client.Search(
		ctx,
		common.MilvusCollectionName,
		r.partition,
		"",
		[]string{"id"},
		[]entity.Vector{entity.BinaryVector(vectorToBytes(vectors[0]))},
		"vector",
		entity.HAMMING,
		topK,
		searchParam,
	)
	if err != nil {
		return nil, fmt.Errorf("search milvus: %w", err)
	}

	hits, err := searchHits(results)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return []*schema.Document{}, nil
	}
	ids := make([]string, len(hits))
	for i := range hits {
		ids[i] = hits[i].ID
	}

	columns, err := r.client.Get(
		ctx,
		common.MilvusCollectionName,
		entity.NewColumnVarChar("id", ids),
		milvusClient.GetWithOutputFields("id", "content", "metadata"),
	)
	if err != nil {
		return nil, fmt.Errorf("query milvus documents: %w", err)
	}
	return documentsForHits(columns, hits, common.MilvusCollectionName)
}

type searchHit struct {
	ID       string
	Distance float64
	Rank     int
}

func searchHits(results []milvusClient.SearchResult) ([]searchHit, error) {
	var hits []searchHit
	for _, result := range results {
		if result.Err != nil {
			return nil, fmt.Errorf("search result: %w", result.Err)
		}
		if result.IDs == nil {
			continue
		}
		if len(result.Scores) != result.IDs.Len() {
			return nil, fmt.Errorf("search result score count mismatch: ids=%d scores=%d", result.IDs.Len(), len(result.Scores))
		}
		for i := 0; i < result.IDs.Len(); i++ {
			id, err := result.IDs.GetAsString(i)
			if err != nil {
				return nil, fmt.Errorf("read search id: %w", err)
			}
			if id != "" {
				hits = append(hits, searchHit{
					ID:       id,
					Distance: float64(result.Scores[i]),
					Rank:     len(hits) + 1,
				})
			}
		}
	}
	return hits, nil
}

func documentsForHits(columns []entity.Column, hits []searchHit, collection string) ([]*schema.Document, error) {
	documents, err := columnsToDocuments(columns)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*schema.Document, len(documents))
	for _, document := range documents {
		byID[document.ID] = document
	}

	ordered := make([]*schema.Document, 0, len(hits))
	for _, hit := range hits {
		document, ok := byID[hit.ID]
		if !ok {
			return nil, fmt.Errorf("document %q missing from Milvus get result", hit.ID)
		}
		if document.MetaData == nil {
			document.MetaData = map[string]any{}
		}
		document.MetaData["_retrieval_distance"] = hit.Distance
		document.MetaData["_retrieval_rank"] = hit.Rank
		document.MetaData["_retrieval_collection"] = collection
		ordered = append(ordered, document)
	}
	return ordered, nil
}

func columnsToDocuments(columns []entity.Column) ([]*schema.Document, error) {
	if len(columns) == 0 {
		return []*schema.Document{}, nil
	}
	length := columns[0].Len()
	documents := make([]*schema.Document, length)
	for i := range documents {
		documents[i] = &schema.Document{MetaData: map[string]any{}}
	}

	for _, column := range columns {
		if column.Len() != length {
			return nil, fmt.Errorf("column %s length mismatch", column.Name())
		}
		for i := 0; i < column.Len(); i++ {
			value, err := column.GetAsString(i)
			if err != nil {
				return nil, fmt.Errorf("read column %s: %w", column.Name(), err)
			}
			switch column.Name() {
			case "id":
				documents[i].ID = value
			case "content":
				documents[i].Content = value
			case "metadata":
				if value != "" {
					if err := json.Unmarshal([]byte(value), &documents[i].MetaData); err != nil {
						return nil, fmt.Errorf("parse metadata: %w", err)
					}
				}
			}
		}
	}
	return documents, nil
}

func vectorToBytes(vector []float64) []byte {
	bytes := make([]byte, len(vector)*4)
	for i, value := range vector {
		bits := math.Float32bits(float32(value))
		bytes[i*4] = byte(bits)
		bytes[i*4+1] = byte(bits >> 8)
		bytes[i*4+2] = byte(bits >> 16)
		bytes[i*4+3] = byte(bits >> 24)
	}
	return bytes
}
