package auth

import (
	"context"
	"testing"
	"time"

	"SuperBizAgent/api/auth/v1"
	authn "SuperBizAgent/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

func testController(t *testing.T) *Controller {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	service, err := authn.NewService(authn.Config{
		Issuer:         "test",
		Secret:         "this-secret-is-long-enough-for-tests",
		AccessTokenTTL: time.Hour,
		Users:          []authn.LocalUser{{Username: "alice", PasswordHash: string(hash), UserID: "u1", TenantID: "t1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Controller{service: service}
}

func TestLoginReturnsBearerTokenWithoutPassword(t *testing.T) {
	controller := testController(t)
	response, err := controller.Login(context.Background(), &v1.LoginReq{Username: "alice", Password: "correct-password"})
	if err != nil {
		t.Fatal(err)
	}
	if response.TokenType != "Bearer" || response.AccessToken == "" || response.ExpiresIn <= 0 {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.UserID != "u1" || response.TenantID != "t1" {
		t.Fatalf("unexpected identity: %#v", response)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	controller := testController(t)
	if _, err := controller.Login(context.Background(), &v1.LoginReq{Username: "alice", Password: "wrong"}); err == nil {
		t.Fatal("expected invalid credentials error")
	}
}
