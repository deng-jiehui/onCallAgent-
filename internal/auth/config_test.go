package auth

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestLoadConfigFromEnv(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	setEnv(t, map[string]string{
		"SUPERBIZ_JWT_SECRET":         "this-local-secret-is-long-enough-123",
		"SUPERBIZ_AUTH_PASSWORD_HASH": string(hash),
		"SUPERBIZ_AUTH_USERNAME":      "admin",
		"SUPERBIZ_AUTH_USER_ID":       "user-admin",
		"SUPERBIZ_AUTH_TENANT_ID":     "tenant-local",
		"SUPERBIZ_AUTH_ROLES":         "operator,admin",
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Issuer != "superbizagent-local" || cfg.AccessTokenTTL <= 0 || len(cfg.Users) != 1 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if !strings.EqualFold(cfg.Users[0].Username, "admin") || len(cfg.Users[0].Roles) != 2 {
		t.Fatalf("unexpected user config: %#v", cfg.Users[0])
	}
}

func TestLoadConfigRequiresSecretAndPasswordHash(t *testing.T) {
	setEnv(t, map[string]string{
		"SUPERBIZ_JWT_SECRET":         "",
		"SUPERBIZ_AUTH_PASSWORD_HASH": "",
	})
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected missing auth environment to fail")
	}
}

func setEnv(t *testing.T, values map[string]string) {
	t.Helper()
	keys := []string{
		"SUPERBIZ_JWT_SECRET", "SUPERBIZ_AUTH_PASSWORD_HASH", "SUPERBIZ_AUTH_USERNAME",
		"SUPERBIZ_AUTH_USER_ID", "SUPERBIZ_AUTH_TENANT_ID", "SUPERBIZ_AUTH_ROLES",
	}
	for _, key := range keys {
		old, exists := os.LookupEnv(key)
		if exists {
			t.Setenv(key, old)
		} else {
			t.Setenv(key, "")
		}
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}
