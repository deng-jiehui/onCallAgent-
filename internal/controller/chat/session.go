package chat

import (
	"context"
	"errors"

	authn "SuperBizAgent/internal/auth"
	"SuperBizAgent/internal/conversation"
)

var errUnauthenticated = errors.New("authenticated principal is required")

func sessionKey(ctx context.Context, conversationID string) (conversation.SessionKey, error) {
	principal, ok := authn.PrincipalFromContext(ctx)
	if !ok {
		return conversation.SessionKey{}, errUnauthenticated
	}
	key := conversation.SessionKey{
		TenantID:       principal.TenantID,
		UserID:         principal.UserID,
		ConversationID: conversationID,
	}
	if err := key.Validate(); err != nil {
		return conversation.SessionKey{}, err
	}
	return key, nil
}
