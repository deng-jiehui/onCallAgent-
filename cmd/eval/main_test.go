package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWritesOfflineReportAtomically(t *testing.T) {
	dir := t.TempDir()
	dataset := filepath.Join(dir, "dataset.jsonl")
	output := filepath.Join(dir, "results", "report.json")
	if err := os.WriteFile(dataset, []byte("{\"id\":\"case-1\",\"question\":\"question\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-dataset", dataset, "-output", output}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run code=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("report not written: %v", err)
	}
	if _, err := os.Stat(output + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary report was not cleaned up: %v", err)
	}
}

func TestRunReturnsTwoForInvalidDataset(t *testing.T) {
	dir := t.TempDir()
	dataset := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(dataset, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-dataset", dataset, "-output", filepath.Join(dir, "report.json")}, &stdout, &stderr); code != 2 {
		t.Fatalf("want exit code 2, got %d", code)
	}
}
