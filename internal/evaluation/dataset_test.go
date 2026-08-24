package evaluation

import (
	"strings"
	"testing"
)

func TestLoadJSONLAcceptsChineseAndOptionalFields(t *testing.T) {
	input := strings.NewReader("{\"id\":\"alert-1\",\"question\":\"服务为什么下线？\",\"tags\":[\"alert\"]}\n")
	dataset, err := LoadJSONL(input)
	if err != nil {
		t.Fatalf("LoadJSONL returned error: %v", err)
	}
	if len(dataset.Cases) != 1 || dataset.Cases[0].Question != "服务为什么下线？" {
		t.Fatalf("unexpected dataset: %#v", dataset)
	}
}

func TestLoadJSONLRejectsDuplicateIDs(t *testing.T) {
	input := strings.NewReader("{\"id\":\"same\",\"question\":\"one\"}\n{\"id\":\"same\",\"question\":\"two\"}\n")
	if _, err := LoadJSONL(input); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("want duplicate error with line number, got %v", err)
	}
}

func TestLoadJSONLRejectsMissingRequiredFields(t *testing.T) {
	tests := []string{
		"{\"id\":\"\",\"question\":\"question\"}\n",
		"{\"id\":\"case\",\"question\":\"\"}\n",
	}
	for _, input := range tests {
		if _, err := LoadJSONL(strings.NewReader(input)); err == nil {
			t.Fatalf("expected validation error for %s", input)
		}
	}
}

func TestLoadJSONLReportsMalformedLine(t *testing.T) {
	input := strings.NewReader("{\"id\":\"ok\",\"question\":\"ok\"}\nnot-json\n")
	if _, err := LoadJSONL(input); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("want malformed JSON line, got %v", err)
	}
}
