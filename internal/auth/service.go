package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
)

type Service struct {
	issuer         string
	secret         []byte
	accessTokenTTL time.Duration
	users          map[string]LocalUser
}

func (s *Service) AccessTokenTTL() time.Duration {
	return s.accessTokenTTL
}

type claims struct {
	TenantID string   `json:"tenant_id"`
	Username string   `json:"username"`
	Roles    []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

func NewService(cfg Config) (*Service, error) {
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, errors.New("auth issuer is required")
	}
	if len(cfg.Secret) < 32 {
		return nil, errors.New("auth secret must be at least 32 characters")
	}
	if cfg.AccessTokenTTL <= 0 {
		return nil, errors.New("access token ttl must be positive")
	}
	users := make(map[string]LocalUser, len(cfg.Users))
	for _, user := range cfg.Users {
		if strings.TrimSpace(user.Username) == "" || user.UserID == "" || user.TenantID == "" {
			return nil, errors.New("local user username, user id, and tenant id are required")
		}
		if _, exists := users[user.Username]; exists {
			return nil, fmt.Errorf("duplicate local user %q", user.Username)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("validation-only")); err != nil {
			if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
				// A valid bcrypt hash is not expected to match this sentinel password.
				// Other errors indicate malformed configuration.
				if _, parseErr := bcrypt.Cost([]byte(user.PasswordHash)); parseErr != nil {
					return nil, fmt.Errorf("invalid password hash for user %q: %w", user.Username, parseErr)
				}
			} else {
				return nil, fmt.Errorf("invalid password hash for user %q: %w", user.Username, err)
			}
		}
		user.Roles = append([]string(nil), user.Roles...)
		users[user.Username] = user
	}
	return &Service{
		issuer:         cfg.Issuer,
		secret:         []byte(cfg.Secret),
		accessTokenTTL: cfg.AccessTokenTTL,
		users:          users,
	}, nil
}

func (s *Service) Login(_ context.Context, username, password string) (Principal, string, error) {
	user, ok := s.users[username]
	if !ok || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return Principal{}, "", ErrInvalidCredentials
	}
	principal := Principal{
		UserID:   user.UserID,
		TenantID: user.TenantID,
		Username: user.Username,
		Roles:    append([]string(nil), user.Roles...),
	}
	token, err := s.issue(principal, time.Now())
	if err != nil {
		return Principal{}, "", err
	}
	return principal, token, nil
}

func (s *Service) Issue(_ context.Context, principal Principal) (string, error) {
	if principal.UserID == "" || principal.TenantID == "" || principal.Username == "" {
		return "", errors.New("principal user id, tenant id, and username are required")
	}
	return s.issue(principal, time.Now())
}

func (s *Service) issue(principal Principal, now time.Time) (string, error) {
	jti, err := randomID()
	if err != nil {
		return "", fmt.Errorf("generate token id: %w", err)
	}
	claims := claims{
		TenantID: principal.TenantID,
		Username: principal.Username,
		Roles:    append([]string(nil), principal.Roles...),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   principal.UserID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *Service) Validate(_ context.Context, rawToken string) (Principal, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return Principal{}, ErrInvalidToken
	}
	parsed, err := jwt.ParseWithClaims(rawToken, &claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Principal{}, ErrTokenExpired
		}
		return Principal{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	validated, ok := parsed.Claims.(*claims)
	if !ok || !parsed.Valid || validated.Subject == "" || validated.TenantID == "" || validated.Username == "" {
		return Principal{}, ErrInvalidToken
	}
	return Principal{
		UserID:   validated.Subject,
		TenantID: validated.TenantID,
		Username: validated.Username,
		Roles:    append([]string(nil), validated.Roles...),
	}, nil
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
