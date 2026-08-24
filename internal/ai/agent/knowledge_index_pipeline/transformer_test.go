package knowledge_index_pipeline

import "testing"

func TestStableChunkID(t *testing.T) {
	first := stableChunkID("告警处理手册.md", 2)
	if first != stableChunkID("告警处理手册.md", 2) {
		t.Fatal("stable chunk ID changed for the same input")
	}
	if first == stableChunkID("告警处理手册.md", 3) {
		t.Fatal("stable chunk ID ignored the split index")
	}
	if len(first) != 64 {
		t.Fatalf("want SHA-256 hex ID, got %q", first)
	}
}

func TestStableChunkIDNormalizesPathSeparators(t *testing.T) {
	windows := stableChunkID(`runbooks\alerts.md`, 0)
	unix := stableChunkID("runbooks/alerts.md", 0)
	if windows != unix {
		t.Fatalf("path separators changed the ID: %q != %q", windows, unix)
	}
}
