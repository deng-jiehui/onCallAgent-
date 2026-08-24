package v1

import "github.com/gogf/gf/v2/frame/g"

type LoginReq struct {
	g.Meta   `path:"/login" method:"post" summary:"本地账号登录"`
	Username string `json:"username" v:"required"`
	Password string `json:"password" v:"required"`
}

type LoginRes struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	UserID      string `json:"user_id"`
	TenantID    string `json:"tenant_id"`
}
