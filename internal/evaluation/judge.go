package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type JudgeInput struct {
	Question  string   `json:"question"`
	Answer    string   `json:"answer"`
	Reference string   `json:"reference"`
	Contexts  []string `json:"contexts,omitempty"`
}

type JudgeScore struct {
	Relevance    float64 `json:"relevance"`
	Faithfulness float64 `json:"faithfulness"`
	Completeness float64 `json:"completeness"`
	Reason       string  `json:"reason"`
}

type Judge interface {
	Evaluate(context.Context, JudgeInput) (JudgeScore, error)
}

type ModelJudge struct {
	model model.BaseChatModel
}

func NewModelJudge(chatModel model.BaseChatModel) *ModelJudge {
	return &ModelJudge{model: chatModel}
}

func (j *ModelJudge) Evaluate(ctx context.Context, input JudgeInput) (JudgeScore, error) {
	if j == nil || j.model == nil {
		return JudgeScore{}, fmt.Errorf("judge model is not configured")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return JudgeScore{}, fmt.Errorf("marshal judge input: %w", err)
	}
	messages := []*schema.Message{
		schema.SystemMessage("Score relevance, faithfulness, and completeness from 0 to 1. Return exactly one JSON object with keys relevance, faithfulness, completeness, and reason. Return no markdown or prose."),
		schema.UserMessage(string(payload)),
	}
	response, err := j.model.Generate(ctx, messages)
	if err != nil {
		return JudgeScore{}, fmt.Errorf("judge model: %w", err)
	}
	if response == nil {
		return JudgeScore{}, fmt.Errorf("judge model returned no message")
	}
	return parseJudgeScore(response.Content)
}

func parseJudgeScore(content string) (JudgeScore, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	var score JudgeScore
	if err := decoder.Decode(&score); err != nil {
		return JudgeScore{}, fmt.Errorf("decode judge JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return JudgeScore{}, fmt.Errorf("judge output must contain exactly one JSON object")
	}
	if score.Relevance < 0 || score.Relevance > 1 || score.Faithfulness < 0 || score.Faithfulness > 1 || score.Completeness < 0 || score.Completeness > 1 {
		return JudgeScore{}, fmt.Errorf("judge scores must be between 0 and 1")
	}
	if strings.TrimSpace(score.Reason) == "" {
		return JudgeScore{}, fmt.Errorf("judge reason is required")
	}
	return score, nil
}
