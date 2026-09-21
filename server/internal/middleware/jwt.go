package middleware

import (
	"net/http"
	"strings"

	"blog/server/internal/resp"
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// gin context 中存放 JWT claims 的 key。
const (
	CtxUserID   = "ctx_user_id"
	CtxUsername = "ctx_username"
)

// JWTAuth Bearer 令牌校验中间件：解析失败一律 401（前端据此跳转登录）。
// 通过后把 user id / username 写入 gin context。
func JWTAuth(auth *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			resp.Abort(c, http.StatusUnauthorized, "缺少认证令牌")
			return
		}
		claims, err := auth.ParseToken(token)
		if err != nil {
			resp.Abort(c, http.StatusUnauthorized, "登录已过期或令牌无效")
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Next()
	}
}

// bearerToken 从 Authorization 头提取 Bearer 令牌。
func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const prefix = "bearer "
	if len(header) >= len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
