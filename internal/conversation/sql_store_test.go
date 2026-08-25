package conversation

import (
	"strings"
	"testing"
)

func TestNewSQLStoreRequiresDatabase(t *testing.T) {
	if _, err := NewSQLStore(nil, 20); err != ErrSQLStoreRequired {
		t.Fatalf("nil database error = %v", err)
	}
}

func TestPostgreSQLConversationSchemaScopesEveryTableByOwner(t *testing.T) {
	schema := PostgreSQLConversationSchema
	for _, fragment := range []string{
		"PRIMARY KEY (tenant_id, user_id, conversation_id)",
		"FOREIGN KEY (tenant_id, user_id, conversation_id)",
		"message_json JSONB",
		"conversation_messages_owner_order_idx",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing %q", fragment)
		}
	}
}
