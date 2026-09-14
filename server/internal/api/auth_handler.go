package api

import (
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler 登录接口。
type AuthHandler struct {
	svc *service.AuthService
}

// NewAuthHandler 创建登录 handler。
func NewAuthHandler(svc *service.AuthService) *AuthHandler { return &AuthHandler{svc: svc} }

// Login POST /auth/login：校验账号密码，返回 {token, user}。
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	token, user, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		failWith(c, err)
		return
	}
	ok(c, gin.H{"token": token, "user": user})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
