-- Conversation ownership is always scoped by tenant, user, and conversation.
-- Apply this migration before constructing conversation.NewSQLStore.
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
