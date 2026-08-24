package evaluation

import (
	"math"
	"testing"
)

func TestRetrievalMetrics(t *testing.T) {
	retrieved := []string{"b", "a", "c"}
	relevant := []string{"a", "c"}
	if got := RecallAtK(retrieved, relevant, 2); got != 0.5 {
		t.Fatalf("Recall@2=%v, want 0.5", got)
	}
	if got := MRR(retrieved, relevant); got != 0.5 {
		t.Fatalf("MRR=%v, want 0.5", got)
	}
	wantNDCG := (1 / math.Log2(3)) / (1 + 1/math.Log2(3))
	if got := NDCGAtK(retrieved, relevant, 2); math.Abs(got-wantNDCG) > 1e-12 {
		t.Fatalf("nDCG@2=%v, want %v", got, wantNDCG)
	}
}

func TestRetrievalMetricsDeduplicateRetrievedIDs(t *testing.T) {
	if got := RecallAtK([]string{"a", "a", "b"}, []string{"a", "b"}, 2); got != 1 {
		t.Fatalf("duplicate ID changed ranking: %v", got)
	}
}

func TestRetrievalMetricsEmptyInputs(t *testing.T) {
	if RecallAtK(nil, nil, 3) != 0 || MRR(nil, nil) != 0 || NDCGAtK(nil, nil, 3) != 0 {
		t.Fatal("empty retrieval metrics must be zero")
	}
}

func TestExactMatchNormalizesCaseWhitespaceAndPunctuation(t *testing.T) {
	if got := ExactMatch(" 服务 DOWN！ ", "服务 down"); got != 1 {
		t.Fatalf("ExactMatch=%v, want 1", got)
	}
}

func TestTokenF1SupportsChineseAndEnglish(t *testing.T) {
	score := TokenF1("服务 down", "服务 up")
	if math.Abs(score.Precision-2.0/3.0) > 1e-12 || math.Abs(score.Recall-2.0/3.0) > 1e-12 || math.Abs(score.F1-2.0/3.0) > 1e-12 {
		t.Fatalf("unexpected Token F1: %#v", score)
	}
}

func TestToolSetAccuracyIgnoresOrderButRequiresExactSet(t *testing.T) {
	if ToolSetAccuracy([]string{"logs", "docs"}, []string{"docs", "logs"}) != 1 {
		t.Fatal("tool order should not matter")
	}
	if ToolSetAccuracy([]string{"docs", "extra"}, []string{"docs"}) != 0 {
		t.Fatal("extra tool should fail exact set accuracy")
	}
}
