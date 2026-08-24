package auth

import (
	"errors"
	"os"
	"strings"
	"time"
)

func LoadConfig() (Config, error) {
	secret := strings.TrimSpace(os.Getenv("SUPERBIZ_JWT_SECRET"))
	passwordHash := strings.TrimSpace(os.Getenv("SUPERBIZ_AUTH_PASSWORD_HASH"))
	if secret == "" || passwordHash == "" {
		return Config{}, errors.New("SUPERBIZ_JWT_SECRET and SUPERBIZ_AUTH_PASSWORD_HASH are required")
	}

	ttl := time.Hour
	if raw := strings.TrimSpace(os.Getenv("SUPERBIZ_JWT_TTL")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return Config{}, errors.New("SUPERBIZ_JWT_TTL must be a positive duration")
		}
		ttl = parsed
	}

	username := strings.TrimSpace(os.Getenv("SUPERBIZ_AUTH_USERNAME"))
	if username == "" {
		username = "admin"
	}
	userID := strings.TrimSpace(os.Getenv("SUPERBIZ_AUTH_USER_ID"))
	if userID == "" {
		userID = "local-admin"
	}
	tenantID := strings.TrimSpace(os.Getenv("SUPERBIZ_AUTH_TENANT_ID"))
	if tenantID == "" {
		tenantID = "local-tenant"
	}
	roles := splitRoles(os.Getenv("SUPERBIZ_AUTH_ROLES"))
	if len(roles) == 0 {
		roles = []string{"operator"}
	}
	return Config{
		Issuer:         "superbizagent-local",
		Secret:         secret,
		AccessTokenTTL: ttl,
		Users: []LocalUser{{
			Username:     username,
			PasswordHash: passwordHash,
			UserID:       userID,
			TenantID:     tenantID,
			Roles:        roles,
		}},
	}, nil
}

func splitRoles(raw string) []string {
	parts := strings.Split(raw, ",")
	roles := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		role := strings.TrimSpace(part)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles
}
