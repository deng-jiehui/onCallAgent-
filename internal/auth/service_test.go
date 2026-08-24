package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		Issuer:         "superbizagent-local",
		Secret:         "local-development-secret-that-is-long-enough",
		AccessTokenTTL: time.Hour,
		Users: []LocalUser{{
			Username:     "alice",
			PasswordHash: string(hash),
			UserID:       "user-alice",
			TenantID:     "tenant-acme",
			Roles:        []string{"operator"},
		}},
	}
}

func TestServiceLoginAndValidateRoundTrip(t *testing.T) {
	service, err := NewService(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}

	want := Principal{UserID: "user-alice", TenantID: "tenant-acme", Username: "alice", Roles: []string{"operator"}}
	principal, token, err := service.Login(context.Background(), "alice", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if principal.UserID != want.UserID || principal.TenantID != want.TenantID || principal.Username != want.Username {
		t.Fatalf("principal = %#v, want %#v", principal, want)
	}

	got, err := service.Validate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != want.UserID || got.TenantID != want.TenantID || got.Username != want.Username {
		t.Fatalf("validated principal = %#v, want %#v", got, want)
	}
}

func TestServiceRejectsInvalidCredentials(t *testing.T) {
	service, err := NewService(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Login(context.Background(), "alice", "wrong-password"); err == nil {
		t.Fatal("expected wrong password to fail")
	}
	if _, _, err := service.Login(context.Background(), "missing", "correct-password"); err == nil {
		t.Fatal("expected unknown user to fail")
	}
}

func TestServiceRejectsExpiredAndTamperedTokens(t *testing.T) {
	cfg := testConfig(t)
	service, err := NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	expired := claims{
		TenantID: "tenant-acme",
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			Subject:   "user-alice",
			ID:        "expired",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	expiredToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, expired).SignedString([]byte(cfg.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Validate(context.Background(), expiredToken); err != ErrTokenExpired {
		t.Fatalf("expired token error = %v, want %v", err, ErrTokenExpired)
	}

	service, err = NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := service.Login(context.Background(), "alice", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Validate(context.Background(), token+"tampered"); err == nil {
		t.Fatal("expected tampered token to fail")
	}

	wrongIssuer := claims{
		TenantID: "tenant-acme",
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "other-issuer",
			Subject:   "user-alice",
			ID:        "wrong-issuer",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	wrongIssuerToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, wrongIssuer).SignedString([]byte(cfg.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Validate(context.Background(), wrongIssuerToken); err == nil {
		t.Fatal("expected issuer mismatch to fail")
	}

	algToken := jwt.NewWithClaims(jwt.SigningMethodHS384, claims{
		TenantID: "tenant-acme",
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			Subject:   "user-alice",
			ID:        "wrong-algorithm",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	wrongAlgorithmToken, err := algToken.SignedString([]byte(cfg.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Validate(context.Background(), wrongAlgorithmToken); err == nil {
		t.Fatal("expected signing algorithm mismatch to fail")
	}
}

func TestPrincipalContextRoundTrip(t *testing.T) {
	want := Principal{UserID: "u1", TenantID: "t1", Username: "alice"}
	got, ok := PrincipalFromContext(WithPrincipal(context.Background(), want))
	if !ok || got.UserID != want.UserID || got.TenantID != want.TenantID {
		t.Fatalf("principal = %#v, ok=%v", got, ok)
	}
}
