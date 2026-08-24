package chat

import (
	"context"
	"testing"

	authn "SuperBizAgent/internal/auth"
)

func TestSessionKeyUsesAuthenticatedIdentity(t *testing.T) {
	ctxA := authn.WithPrincipal(context.Background(), authn.Principal{TenantID: "tenant-a", UserID: "user-a"})
	ctxB := authn.WithPrincipal(context.Background(), authn.Principal{TenantID: "tenant-a", UserID: "user-b"})

	keyA, err := sessionKey(ctxA, "same-conversation")
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := sessionKey(ctxB, "same-conversation")
	if err != nil {
		t.Fatal(err)
	}
	if keyA == keyB {
		t.Fatalf("different users share session key: %#v", keyA)
	}
	if keyA.TenantID != "tenant-a" || keyA.UserID != "user-a" || keyA.ConversationID != "same-conversation" {
		t.Fatalf("unexpected session key: %#v", keyA)
	}
}

func TestSessionKeyRejectsMissingIdentityOrConversation(t *testing.T) {
	if _, err := sessionKey(context.Background(), "conversation"); err == nil {
		t.Fatal("expected missing principal to fail")
	}
	ctx := authn.WithPrincipal(context.Background(), authn.Principal{TenantID: "tenant", UserID: "user"})
	if _, err := sessionKey(ctx, ""); err == nil {
		t.Fatal("expected empty conversation id to fail")
	}
}
