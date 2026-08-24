package evaluation

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

func TestRunAggregatesEligibleMetricsAndTools(t *testing.T) {
	dataset := Dataset{Cases: []Case{{
		ID: "case-1", Question: "question", RelevantDocIDs: []string{"doc-a"},
		ReferenceAnswer: "service down", ExpectedTools: []string{"query_internal_docs"}, Tags: []string{"alert"},
	}}}
	deps := Dependencies{
		Retrieve: func(context.Context, string, int) ([]*schema.Document, error) {
			return []*schema.Document{{ID: "doc-a", MetaData: map[string]any{"_retrieval_distance": float64(8), "_retrieval_rank": 1}}}, nil
		},
		Agent: func(ctx context.Context, _ string, handler callbacks.Handler) (*schema.Message, error) {
			info := &callbacks.RunInfo{Name: "query_internal_docs", Type: "Tool", Component: components.ComponentOfTool}
			callbackCtx := handler.OnStart(ctx, info, &tool.CallbackInput{ArgumentsInJSON: `{}`})
			handler.OnEnd(callbackCtx, info, &tool.CallbackOutput{Response: "ok"})
			return schema.AssistantMessage("service down", nil), nil
		},
	}

	report := Run(context.Background(), dataset, Config{TopK: 5, RunRetrieval: true, RunAgent: true, Tags: []string{"alert"}}, deps)
	if len(report.Cases) != 1 || len(report.Cases[0].Errors) != 0 {
		t.Fatalf("unexpected report cases: %#v", report.Cases)
	}
	if report.Cases[0].Tools[0] != "query_internal_docs" {
		t.Fatalf("tool call not recorded: %#v", report.Cases[0].Tools)
	}
	if got := report.Aggregates["recall_at_k"]; got.Count != 1 || got.Mean != 1 {
		t.Fatalf("unexpected recall aggregate: %#v", got)
	}
	if got := report.Aggregates["tool_set_accuracy"]; got.Count != 1 || got.Mean != 1 {
		t.Fatalf("unexpected tool aggregate: %#v", got)
	}
}

func TestRunKeepsAgentMetricsWhenRetrievalFails(t *testing.T) {
	dataset := Dataset{Cases: []Case{{ID: "case-1", Question: "q", RelevantDocIDs: []string{"doc"}, ReferenceAnswer: "answer"}}}
	report := Run(context.Background(), dataset, Config{TopK: 1, RunRetrieval: true, RunAgent: true}, Dependencies{
		Retrieve: func(context.Context, string, int) ([]*schema.Document, error) {
			return nil, errors.New("milvus unavailable")
		},
		Agent: func(context.Context, string, callbacks.Handler) (*schema.Message, error) {
			return schema.AssistantMessage("answer", nil), nil
		},
	})
	if len(report.Cases[0].Errors) != 1 {
		t.Fatalf("want one retrieval error, got %#v", report.Cases[0].Errors)
	}
	if report.Aggregates["recall_at_k"].Count != 0 || report.Aggregates["exact_match"].Count != 1 {
		t.Fatalf("eligible denominators are wrong: %#v", report.Aggregates)
	}
}

func TestRunFiltersTagsAndPreservesDatasetOrder(t *testing.T) {
	dataset := Dataset{Cases: []Case{
		{ID: "first", Question: "q1", Tags: []string{"keep"}},
		{ID: "second", Question: "q2", Tags: []string{"skip"}},
		{ID: "third", Question: "q3", Tags: []string{"keep"}},
	}}
	report := Run(context.Background(), dataset, Config{Tags: []string{"keep"}}, Dependencies{})
	if len(report.Cases) != 2 || report.Cases[0].ID != "first" || report.Cases[1].ID != "third" {
		t.Fatalf("tag filtering changed order: %#v", report.Cases)
	}
}
