package evaluation

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeJudgeModel struct {
	content string
}

func (f fakeJudgeModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage(f.content, nil), nil
}

func (f fakeJudgeModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage(f.content, nil)}), nil
}

func TestModelJudgeParsesStrictJSON(t *testing.T) {
	judge := NewModelJudge(fakeJudgeModel{content: `{"relevance":0.9,"faithfulness":0.8,"completeness":0.7,"reason":"grounded"}`})
	score, err := judge.Evaluate(context.Background(), JudgeInput{Question: "q", Answer: "a", Reference: "r"})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if score.Relevance != 0.9 || score.Faithfulness != 0.8 || score.Completeness != 0.7 || score.Reason != "grounded" {
		t.Fatalf("unexpected score: %#v", score)
	}
}

func TestModelJudgeRejectsInvalidOutput(t *testing.T) {
	outputs := []string{
		`result: {"relevance":0.9,"faithfulness":0.8,"completeness":0.7,"reason":"x"}`,
		`{"relevance":1.2,"faithfulness":0.8,"completeness":0.7,"reason":"x"}`,
		`{"relevance":0.9,"faithfulness":0.8,"completeness":0.7,"reason":""}`,
	}
	for _, output := range outputs {
		judge := NewModelJudge(fakeJudgeModel{content: output})
		if _, err := judge.Evaluate(context.Background(), JudgeInput{}); err == nil || strings.TrimSpace(err.Error()) == "" {
			t.Fatalf("expected invalid judge output error for %q", output)
		}
	}
}
