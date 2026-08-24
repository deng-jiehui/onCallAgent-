package auth

import (
	"context"
	"errors"

	api "SuperBizAgent/api/auth"
	"SuperBizAgent/api/auth/v1"
	authn "SuperBizAgent/internal/auth"
)

type Controller struct {
	service *authn.Service
}

func New(service *authn.Service) api.IAuthV1 {
	return &Controller{service: service}
}

func (c *Controller) Login(ctx context.Context, req *v1.LoginReq) (*v1.LoginRes, error) {
	if req == nil {
		return nil, errors.New("login request is required")
	}
	principal, token, err := c.service.Login(ctx, req.Username, req.Password)
	if err != nil {
		return nil, err
	}
	return &v1.LoginRes{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(c.service.AccessTokenTTL().Seconds()),
		UserID:      principal.UserID,
		TenantID:    principal.TenantID,
	}, nil
}
