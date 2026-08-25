package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

var ErrSQLStoreRequired = errors.New("conversation SQL store database is required")

// SQLStore persists conversation history in PostgreSQL. The database handle
// and driver lifecycle are owned by the application; this store only owns
// transactions issued for conversation reads and writes.
type SQLStore struct {
	db          *sql.DB
	maxMessages int
}

func NewSQLStore(db *sql.DB, maxMessages int) (*SQLStore, error) {
	if db == nil {
		return nil, ErrSQLStoreRequired
	}
	if maxMessages < 2 {
		maxMessages = 20
	}
	if maxMessages%2 != 0 {
		maxMessages--
	}
	return &SQLStore{db: db, maxMessages: maxMessages}, nil
}

func (s *SQLStore) Load(ctx context.Context, key SessionKey) ([]*schema.Message, int64, error) {
	ctx = normalizeContext(ctx)
	if err := key.Validate(); err != nil {
		return nil, 0, err
	}
	var version int64
	err := s.db.QueryRowContext(ctx, `
		SELECT version FROM conversations
		WHERE tenant_id = $1 AND user_id = $2 AND conversation_id = $3`,
		key.TenantID, key.UserID, key.ConversationID).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT message_json FROM conversation_messages
		WHERE tenant_id = $1 AND user_id = $2 AND conversation_id = $3
		ORDER BY sequence_no`, key.TenantID, key.UserID, key.ConversationID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var messages []*schema.Message
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, err
		}
		var message schema.Message
		if err := json.Unmarshal(raw, &message); err != nil {
			return nil, 0, fmt.Errorf("decode conversation message: %w", err)
		}
		messages = append(messages, &message)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return messages, version, nil
}

func (s *SQLStore) Append(ctx context.Context, key SessionKey, userMessage, assistantMessage *schema.Message) error {
	_, err := s.append(ctx, key, 0, userMessage, assistantMessage, false)
	return err
}

func (s *SQLStore) AppendIfVersion(ctx context.Context, key SessionKey, expectedVersion int64, userMessage, assistantMessage *schema.Message) (bool, error) {
	return s.append(ctx, key, expectedVersion, userMessage, assistantMessage, true)
}

func (s *SQLStore) AppendIdempotent(ctx context.Context, key SessionKey, idempotencyKey string, userMessage, assistantMessage *schema.Message) (bool, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return false, errors.New("idempotency key is required")
	}
	return s.appendWithKey(ctx, key, 0, userMessage, assistantMessage, false, idempotencyKey)
}

func (s *SQLStore) append(ctx context.Context, key SessionKey, expectedVersion int64, userMessage, assistantMessage *schema.Message, checkVersion bool) (bool, error) {
	return s.appendWithKey(ctx, key, expectedVersion, userMessage, assistantMessage, checkVersion, "")
}

func (s *SQLStore) appendWithKey(ctx context.Context, key SessionKey, expectedVersion int64, userMessage, assistantMessage *schema.Message, checkVersion bool, idempotencyKey string) (bool, error) {
	ctx = normalizeContext(ctx)
	if err := key.Validate(); err != nil {
		return false, err
	}
	if userMessage == nil || assistantMessage == nil {
		return false, ErrInvalidMessage
	}
	userJSON, err := json.Marshal(userMessage)
	if err != nil {
		return false, fmt.Errorf("encode user message: %w", err)
	}
	assistantJSON, err := json.Marshal(assistantMessage)
	if err != nil {
		return false, fmt.Errorf("encode assistant message: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	rollback := func() { _ = tx.Rollback() }
	defer rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO conversations (tenant_id, user_id, conversation_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, 0, $4, $4)
		ON CONFLICT (tenant_id, user_id, conversation_id) DO NOTHING`,
		key.TenantID, key.UserID, key.ConversationID, time.Now().UTC())
	if err != nil {
		return false, err
	}
	if idempotencyKey != "" {
		hash, hashErr := messagePairHash(userMessage, assistantMessage)
		if hashErr != nil {
			return false, hashErr
		}
		result, insertErr := tx.ExecContext(ctx, `
			INSERT INTO conversation_idempotency
			(tenant_id, user_id, conversation_id, idempotency_key, request_hash, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (tenant_id, user_id, conversation_id, idempotency_key) DO NOTHING`,
			key.TenantID, key.UserID, key.ConversationID, idempotencyKey, hash, time.Now().UTC())
		if insertErr != nil {
			return false, insertErr
		}
		inserted, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return false, rowsErr
		}
		if inserted == 0 {
			var previousHash string
			if err := tx.QueryRowContext(ctx, `
				SELECT request_hash FROM conversation_idempotency
				WHERE tenant_id = $1 AND user_id = $2 AND conversation_id = $3 AND idempotency_key = $4`,
				key.TenantID, key.UserID, key.ConversationID, idempotencyKey).Scan(&previousHash); err != nil {
				return false, err
			}
			if previousHash != hash {
				return false, ErrIdempotencyConflict
			}
			if err := tx.Commit(); err != nil {
				return false, err
			}
			return false, nil
		}
	}
	var version int64
	if err := tx.QueryRowContext(ctx, `
		SELECT version FROM conversations
		WHERE tenant_id = $1 AND user_id = $2 AND conversation_id = $3
		FOR UPDATE`, key.TenantID, key.UserID, key.ConversationID).Scan(&version); err != nil {
		return false, err
	}
	if checkVersion && version != expectedVersion {
		return false, tx.Commit()
	}
	nextVersion := version + 1
	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations SET version = $1, updated_at = $2
		WHERE tenant_id = $3 AND user_id = $4 AND conversation_id = $5`,
		nextVersion, time.Now().UTC(), key.TenantID, key.UserID, key.ConversationID); err != nil {
		return false, err
	}
	for _, raw := range [][]byte{userJSON, assistantJSON} {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO conversation_messages
			(tenant_id, user_id, conversation_id, turn_version, message_json, created_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
			key.TenantID, key.UserID, key.ConversationID, nextVersion, raw, time.Now().UTC()); err != nil {
			return false, err
		}
	}
	if err := s.trimTx(ctx, tx, key); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLStore) trimTx(ctx context.Context, tx *sql.Tx, key SessionKey) error {
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM conversation_messages
		WHERE tenant_id = $1 AND user_id = $2 AND conversation_id = $3`,
		key.TenantID, key.UserID, key.ConversationID).Scan(&count); err != nil {
		return err
	}
	excess := count - s.maxMessages
	if excess <= 0 {
		return nil
	}
	if excess%2 != 0 {
		excess++
	}
	_, err := tx.ExecContext(ctx, `
		DELETE FROM conversation_messages WHERE sequence_no IN (
			SELECT sequence_no FROM conversation_messages
			WHERE tenant_id = $1 AND user_id = $2 AND conversation_id = $3
			ORDER BY sequence_no LIMIT $4
		)`, key.TenantID, key.UserID, key.ConversationID, excess)
	return err
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// PostgreSQLConversationSchema is the migration body for SQLStore.
const PostgreSQLConversationSchema = `
CREATE TABLE IF NOT EXISTS conversations (
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, user_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS conversation_messages (
    sequence_no BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    turn_version BIGINT NOT NULL,
    message_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT conversation_messages_owner_fk
        FOREIGN KEY (tenant_id, user_id, conversation_id)
        REFERENCES conversations (tenant_id, user_id, conversation_id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS conversation_messages_owner_order_idx
    ON conversation_messages (tenant_id, user_id, conversation_id, sequence_no);

CREATE TABLE IF NOT EXISTS conversation_idempotency (
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, user_id, conversation_id, idempotency_key),
    CONSTRAINT conversation_idempotency_owner_fk
        FOREIGN KEY (tenant_id, user_id, conversation_id)
        REFERENCES conversations (tenant_id, user_id, conversation_id)
        ON DELETE CASCADE
);
`

var _ ConversationStore = (*SQLStore)(nil)
var _ IdempotentConversationStore = (*SQLStore)(nil)
