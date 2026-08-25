package tools

import (
	"context"
	"errors"
	"testing"
)

func TestValidateReadOnlySQL(t *testing.T) {
	allowed := []string{"SELECT * FROM alerts", " show tables", "EXPLAIN SELECT 1"}
	for _, query := range allowed {
		if err := validateReadOnlySQL(query); err != nil {
			t.Errorf("allowed query %q rejected: %v", query, err)
		}
	}
	for _, query := range []string{"UPDATE alerts SET status='x'", "DELETE FROM alerts", "SELECT 1; DROP TABLE alerts", "INSERT INTO alerts VALUES (1)"} {
		if err := validateReadOnlySQL(query); err == nil {
			t.Errorf("write or multi-statement query %q was accepted", query)
		}
	}
}

func TestMysqlCrudToolRequiresServerConfiguredDSN(t *testing.T) {
	tool, err := NewMysqlCrudToolWithConfig(MysqlCrudConfig{})
	if err != nil {
		t.Fatalf("construct tool: %v", err)
	}
	if _, err := tool.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`); !errors.Is(err, ErrMysqlDSNRequired) {
		t.Fatalf("missing server DSN error = %v", err)
	}
}
