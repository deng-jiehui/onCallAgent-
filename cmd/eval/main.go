package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/models"
	internalretriever "SuperBizAgent/internal/ai/retriever"
	"SuperBizAgent/internal/evaluation"

	"github.com/cloudwego/eino/callbacks"
	componentretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("superbizagent-eval", flag.ContinueOnError)
	flags.SetOutput(stderr)
	datasetPath := flags.String("dataset", "eval/datasets/smoke.jsonl", "JSONL evaluation dataset")
	outputPath := flags.String("output", "eval/results/latest.json", "JSON report output")
	topK := flags.Int("top-k", 5, "retrieval result count")
	tags := flags.String("tags", "", "comma-separated case tags")
	runRetrieval := flags.Bool("run-retrieval", false, "run real Milvus retrieval")
	runAgent := flags.Bool("run-agent", false, "run the real Eino Agent")
	runJudge := flags.Bool("judge", false, "run the model judge")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *topK <= 0 {
		fmt.Fprintln(stderr, "-top-k must be greater than zero")
		return 2
	}
	if *runJudge && !*runAgent {
		fmt.Fprintln(stderr, "-judge requires -run-agent=true")
		return 2
	}

	file, err := os.Open(*datasetPath)
	if err != nil {
		fmt.Fprintf(stderr, "open dataset: %v\n", err)
		return 2
	}
	dataset, err := evaluation.LoadJSONL(file)
	_ = file.Close()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	ctx := context.Background()
	dependencies := evaluation.Dependencies{}
	if *runRetrieval {
		retriever, createErr := internalretriever.NewMilvusRetriever(ctx)
		if createErr != nil {
			fmt.Fprintf(stderr, "create retriever: %v\n", createErr)
			return 2
		}
		dependencies.Retrieve = func(ctx context.Context, query string, k int) ([]*schema.Document, error) {
			return retriever.Retrieve(ctx, query, componentretriever.WithTopK(k))
		}
	}
	if *runAgent {
		agent, createErr := chat_pipeline.BuildChatAgent(ctx)
		if createErr != nil {
			fmt.Fprintf(stderr, "create agent: %v\n", createErr)
			return 2
		}
		dependencies.Agent = func(ctx context.Context, question string, handler callbacks.Handler) (*schema.Message, error) {
			return agent.Invoke(ctx, &chat_pipeline.UserMessage{ID: "evaluation", Query: question}, compose.WithCallbacks(handler))
		}
	}
	if *runJudge {
		judgeModel, createErr := models.OpenAIForDeepSeekV3Quick(ctx)
		if createErr != nil {
			fmt.Fprintf(stderr, "create judge model: %v\n", createErr)
			return 2
		}
		dependencies.Judge = evaluation.NewModelJudge(judgeModel)
	}

	report := evaluation.Run(ctx, dataset, evaluation.Config{
		TopK: *topK, Tags: splitCSV(*tags), RunRetrieval: *runRetrieval, RunAgent: *runAgent, RunJudge: *runJudge,
	}, dependencies)
	if err := report.Validate(); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err := writeReport(*outputPath, report); err != nil {
		fmt.Fprintf(stderr, "write report: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "evaluated %d cases; failed=%d; report=%s\n", len(report.Cases), report.FailedCases, *outputPath)
	if report.FailedCases > 0 {
		return 1
	}
	return 0
}

func writeReport(path string, report evaluation.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func splitCSV(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
