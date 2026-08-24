package evaluation

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/schema"
)

type RetrieveFunc func(context.Context, string, int) ([]*schema.Document, error)
type AgentFunc func(context.Context, string, callbacks.Handler) (*schema.Message, error)

type Config struct {
	TopK         int      `json:"top_k"`
	Tags         []string `json:"tags,omitempty"`
	RunRetrieval bool     `json:"run_retrieval"`
	RunAgent     bool     `json:"run_agent"`
	RunJudge     bool     `json:"run_judge"`
	Model        string   `json:"model,omitempty"`
	Embedding    string   `json:"embedding,omitempty"`
	Revision     string   `json:"revision,omitempty"`
}

type Dependencies struct {
	Retrieve RetrieveFunc
	Agent    AgentFunc
	Judge    Judge
}

type RetrievedDocument struct {
	ID       string   `json:"id"`
	Rank     int      `json:"rank"`
	Distance *float64 `json:"distance,omitempty"`
}

type CaseMetrics struct {
	RecallAtK       *float64 `json:"recall_at_k,omitempty"`
	MRR             *float64 `json:"mrr,omitempty"`
	NDCGAtK         *float64 `json:"ndcg_at_k,omitempty"`
	ExactMatch      *float64 `json:"exact_match,omitempty"`
	TokenF1         *float64 `json:"token_f1,omitempty"`
	ToolSetAccuracy *float64 `json:"tool_set_accuracy,omitempty"`
}

type CaseResult struct {
	ID        string              `json:"id"`
	Question  string              `json:"question"`
	Retrieved []RetrievedDocument `json:"retrieved,omitempty"`
	Answer    string              `json:"answer,omitempty"`
	Tools     []string            `json:"tools,omitempty"`
	Metrics   CaseMetrics         `json:"metrics"`
	Judge     *JudgeScore         `json:"judge,omitempty"`
	Errors    []string            `json:"errors,omitempty"`
}

type Aggregate struct {
	Mean  float64 `json:"mean"`
	Count int     `json:"count"`
}

type Report struct {
	GeneratedAt time.Time            `json:"generated_at"`
	Config      Config               `json:"config"`
	Cases       []CaseResult         `json:"cases"`
	Aggregates  map[string]Aggregate `json:"aggregates"`
	FailedCases int                  `json:"failed_cases"`
}

func Run(ctx context.Context, dataset Dataset, cfg Config, deps Dependencies) Report {
	if cfg.TopK <= 0 {
		cfg.TopK = 5
	}
	report := Report{
		GeneratedAt: time.Now().UTC(),
		Config:      cfg,
		Aggregates: map[string]Aggregate{
			"recall_at_k": {}, "mrr": {}, "ndcg_at_k": {}, "exact_match": {},
			"token_f1": {}, "tool_set_accuracy": {}, "judge_relevance": {},
			"judge_faithfulness": {}, "judge_completeness": {},
		},
	}
	for _, item := range dataset.Cases {
		if !matchesTags(item.Tags, cfg.Tags) {
			continue
		}
		result := runCase(ctx, item, cfg, deps)
		if len(result.Errors) > 0 {
			report.FailedCases++
		}
		report.Cases = append(report.Cases, result)
		addCaseAggregates(report.Aggregates, result)
	}
	return report
}

func runCase(ctx context.Context, item Case, cfg Config, deps Dependencies) CaseResult {
	result := CaseResult{ID: item.ID, Question: item.Question}
	var retrievedDocs []*schema.Document
	if cfg.RunRetrieval {
		if deps.Retrieve == nil {
			result.Errors = append(result.Errors, "retrieval: dependency is not configured")
		} else {
			docs, err := deps.Retrieve(ctx, item.Question, cfg.TopK)
			if err != nil {
				result.Errors = append(result.Errors, "retrieval: "+err.Error())
			} else {
				retrievedDocs = docs
				result.Retrieved = retrievedDocuments(docs)
				if len(item.RelevantDocIDs) > 0 {
					ids := documentIDs(docs)
					result.Metrics.RecallAtK = float64Pointer(RecallAtK(ids, item.RelevantDocIDs, cfg.TopK))
					result.Metrics.MRR = float64Pointer(MRR(ids, item.RelevantDocIDs))
					result.Metrics.NDCGAtK = float64Pointer(NDCGAtK(ids, item.RelevantDocIDs, cfg.TopK))
				}
			}
		}
	}

	if cfg.RunAgent {
		if deps.Agent == nil {
			result.Errors = append(result.Errors, "agent: dependency is not configured")
		} else {
			recorder := newToolRecorder()
			message, err := deps.Agent(ctx, item.Question, recorder.handler())
			result.Tools = recorder.names()
			if err != nil {
				result.Errors = append(result.Errors, "agent: "+err.Error())
			} else if message == nil {
				result.Errors = append(result.Errors, "agent: empty response")
			} else {
				result.Answer = message.Content
				if item.ReferenceAnswer != "" {
					result.Metrics.ExactMatch = float64Pointer(ExactMatch(result.Answer, item.ReferenceAnswer))
					result.Metrics.TokenF1 = float64Pointer(TokenF1(result.Answer, item.ReferenceAnswer).F1)
				}
				if item.ExpectedTools != nil {
					result.Metrics.ToolSetAccuracy = float64Pointer(ToolSetAccuracy(result.Tools, item.ExpectedTools))
				}
				if cfg.RunJudge {
					if deps.Judge == nil {
						result.Errors = append(result.Errors, "judge: dependency is not configured")
					} else {
						score, judgeErr := deps.Judge.Evaluate(ctx, JudgeInput{
							Question: item.Question, Answer: result.Answer, Reference: item.ReferenceAnswer,
							Contexts: documentContents(retrievedDocs),
						})
						if judgeErr != nil {
							result.Errors = append(result.Errors, "judge: "+judgeErr.Error())
						} else {
							result.Judge = &score
						}
					}
				}
			}
		}
	}
	return result
}

func addCaseAggregates(aggregates map[string]Aggregate, result CaseResult) {
	addAggregate(aggregates, "recall_at_k", result.Metrics.RecallAtK)
	addAggregate(aggregates, "mrr", result.Metrics.MRR)
	addAggregate(aggregates, "ndcg_at_k", result.Metrics.NDCGAtK)
	addAggregate(aggregates, "exact_match", result.Metrics.ExactMatch)
	addAggregate(aggregates, "token_f1", result.Metrics.TokenF1)
	addAggregate(aggregates, "tool_set_accuracy", result.Metrics.ToolSetAccuracy)
	if result.Judge != nil {
		addAggregate(aggregates, "judge_relevance", &result.Judge.Relevance)
		addAggregate(aggregates, "judge_faithfulness", &result.Judge.Faithfulness)
		addAggregate(aggregates, "judge_completeness", &result.Judge.Completeness)
	}
}

func addAggregate(aggregates map[string]Aggregate, name string, value *float64) {
	if value == nil {
		return
	}
	current := aggregates[name]
	current.Mean = (current.Mean*float64(current.Count) + *value) / float64(current.Count+1)
	current.Count++
	aggregates[name] = current
}

type toolRecorder struct {
	mu    sync.Mutex
	tools []string
}

func newToolRecorder() *toolRecorder { return &toolRecorder{} }

func (r *toolRecorder) handler() callbacks.Handler {
	builder := callbacks.NewHandlerBuilder().OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		if info != nil && info.Component == components.ComponentOfTool {
			name := info.Name
			if name == "" {
				name = info.Type
			}
			r.mu.Lock()
			r.tools = append(r.tools, name)
			r.mu.Unlock()
		}
		return ctx
	})
	builder.OnEndFn(func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
		return ctx
	})
	builder.OnErrorFn(func(ctx context.Context, _ *callbacks.RunInfo, _ error) context.Context { return ctx })
	return builder.Build()
}

func (r *toolRecorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.tools...)
}

func retrievedDocuments(docs []*schema.Document) []RetrievedDocument {
	result := make([]RetrievedDocument, 0, len(docs))
	for index, doc := range docs {
		if doc == nil {
			continue
		}
		rank := index + 1
		if value, ok := numericInt(doc.MetaData["_retrieval_rank"]); ok {
			rank = value
		}
		var distance *float64
		if value, ok := numericFloat(doc.MetaData["_retrieval_distance"]); ok {
			distance = float64Pointer(value)
		}
		result = append(result, RetrievedDocument{ID: doc.ID, Rank: rank, Distance: distance})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Rank < result[j].Rank })
	return result
}

func numericFloat(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	default:
		return 0, false
	}
}

func numericInt(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		return int(number), true
	default:
		return 0, false
	}
}

func documentIDs(docs []*schema.Document) []string {
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc != nil {
			ids = append(ids, doc.ID)
		}
	}
	return ids
}

func documentContents(docs []*schema.Document) []string {
	contents := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc != nil {
			contents = append(contents, doc.Content)
		}
	}
	return contents
}

func matchesTags(caseTags, selected []string) bool {
	if len(selected) == 0 {
		return true
	}
	available := stringSet(caseTags)
	for _, tag := range selected {
		if _, ok := available[tag]; ok {
			return true
		}
	}
	return false
}

func float64Pointer(value float64) *float64 { return &value }

func (r Report) Validate() error {
	if len(r.Cases) == 0 {
		return fmt.Errorf("evaluation selected no cases")
	}
	return nil
}
