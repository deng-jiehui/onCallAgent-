package observability

import "testing"

func TestMetricAttributesAreBounded(t *testing.T) {
	attrs := componentAttributes("component", "type", "error", "query-is-not-a-label")
	if len(attrs) != 3 {
		t.Fatalf("unexpected attribute count: %d", len(attrs))
	}
	for _, attr := range attrs {
		if attr.Key == "query" || attr.Key == "request_id" {
			t.Fatalf("high-cardinality attribute leaked into metrics: %s", attr.Key)
		}
	}
}
