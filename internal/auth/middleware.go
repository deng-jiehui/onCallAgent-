package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
)

func (s *Service) Middleware(r *ghttp.Request) {
	token, ok := bearerToken(r.GetHeader("Authorization"))
	if !ok {
		r.Response.WriteStatusExit(http.StatusUnauthorized, "missing or invalid bearer token")
		return
	}
	principal, err := s.Validate(r.GetCtx(), token)
	if err != nil {
		if errors.Is(err, ErrTokenExpired) || errors.Is(err, ErrInvalidToken) {
			r.Response.WriteStatusExit(http.StatusUnauthorized, "invalid or expired token")
			return
		}
		r.Response.WriteStatusExit(http.StatusUnauthorized, "authentication failed")
		return
	}
	r.SetCtx(WithPrincipal(r.GetCtx(), principal))
	r.Middleware.Next()
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
