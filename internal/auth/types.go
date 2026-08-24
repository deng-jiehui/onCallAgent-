package auth

import "time"

type Principal struct {
	UserID   string
	TenantID string
	Username string
	Roles    []string
}

type LocalUser struct {
	Username     string
	PasswordHash string
	UserID       string
	TenantID     string
	Roles        []string
}

type Config struct {
	Issuer         string
	Secret         string
	AccessTokenTTL time.Duration
	Users          []LocalUser
}
