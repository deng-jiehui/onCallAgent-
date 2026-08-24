package auth

import (
	"context"

	"SuperBizAgent/api/auth/v1"
)

type IAuthV1 interface {
	Login(ctx context.Context, req *v1.LoginReq) (*v1.LoginRes, error)
}
