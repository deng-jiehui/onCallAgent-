package retriever

import (
	"context"
	"testing"

	authn "SuperBizAgent/internal/auth"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func TestTenantFilterExpressionUsesAuthenticatedTenant(t *testing.T) {
	ctx := authn.WithPrincipal(context.Background(), authn.Principal{TenantID: "tenant-acme"})
	filter, err := tenantFilterExpression(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if filter != `metadata["tenant_id"] == "tenant-acme"` {
		t.Fatalf("filter = %q", filter)
	}
}

func TestTenantFilterExpressionRejectsMissingTenant(t *testing.T) {
	if _, err := tenantFilterExpression(context.Background()); err == nil {
		t.Fatal("expected missing tenant scope to fail")
	}
}

func TestTenantFilterExpressionEscapesTenantID(t *testing.T) {
	ctx := authn.WithPrincipal(context.Background(), authn.Principal{TenantID: `tenant"quoted`})
	filter, err := tenantFilterExpression(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if filter != `metadata["tenant_id"] == "tenant\"quoted"` {
		t.Fatalf("filter = %q", filter)
	}
}

func TestSearchHitsPreserveDistanceAndRank(t *testing.T) {
	results := []client.SearchResult{{
		IDs:    entity.NewColumnVarChar("id", []string{"doc-b", "doc-a"}),
		Scores: []float32{8, 17},
	}}

	hits, err := searchHits(results)
	if err != nil {
		t.Fatalf("searchHits returned error: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d", len(hits))
	}
	if hits[0].ID != "doc-b" || hits[0].Distance != 8 || hits[0].Rank != 1 {
		t.Fatalf("unexpected first hit: %#v", hits[0])
	}
	if hits[1].ID != "doc-a" || hits[1].Distance != 17 || hits[1].Rank != 2 {
		t.Fatalf("unexpected second hit: %#v", hits[1])
	}
}

func TestSearchHitsRejectMissingDistance(t *testing.T) {
	results := []client.SearchResult{{
		IDs:    entity.NewColumnVarChar("id", []string{"doc-a"}),
		Scores: nil,
	}}

	if _, err := searchHits(results); err == nil {
		t.Fatal("expected score-count mismatch error")
	}
}

func TestDocumentsForHitsRestoresSearchOrderAndMetadata(t *testing.T) {
	columns := []entity.Column{
		entity.NewColumnVarChar("id", []string{"doc-a", "doc-b"}),
		entity.NewColumnVarChar("content", []string{"A", "B"}),
		entity.NewColumnJSONBytes("metadata", [][]byte{[]byte(`{"source":"a"}`), []byte(`{"source":"b"}`)}),
	}
	hits := []searchHit{
		{ID: "doc-b", Distance: 8, Rank: 1},
		{ID: "doc-a", Distance: 17, Rank: 2},
	}

	docs, err := documentsForHits(columns, hits, "biz")
	if err != nil {
		t.Fatalf("documentsForHits returned error: %v", err)
	}
	if docs[0].ID != "doc-b" || docs[1].ID != "doc-a" {
		t.Fatalf("search order was not preserved: %s, %s", docs[0].ID, docs[1].ID)
	}
	if got := docs[0].MetaData["_retrieval_distance"]; got != float64(8) {
		t.Fatalf("unexpected distance metadata: %#v", got)
	}
	if got := docs[0].MetaData["_retrieval_rank"]; got != 1 {
		t.Fatalf("unexpected rank metadata: %#v", got)
	}
	if got := docs[0].MetaData["_retrieval_collection"]; got != "biz" {
		t.Fatalf("unexpected collection metadata: %#v", got)
	}
}

func TestFilterHitsRejectsLowQualityDistances(t *testing.T) {
	hits := []searchHit{{ID: "good", Distance: 2, Rank: 1}, {ID: "bad", Distance: 20, Rank: 2}}
	filtered := filterHits(hits, 10)
	if len(filtered) != 1 || filtered[0].ID != "good" {
		t.Fatalf("filtered hits = %#v", filtered)
	}
	if got := filterHits(hits, 0); len(got) != 2 {
		t.Fatalf("zero threshold should disable filtering: %#v", got)
	}
}

func TestDefaultRetrieverConfigUsesFiveResults(t *testing.T) {
	cfg := defaultRetrieverConfig()
	if cfg.TopK != 5 || cfg.DistanceThreshold != nil {
		t.Fatalf("default retriever config = %#v", cfg)
	}
}
