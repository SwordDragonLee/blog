package api

import (
	"blog/server/internal/httputil"
	"blog/server/internal/model"
	"blog/server/internal/resp"
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler 登录接口。
type AuthHandler struct {
	svc *service.AuthService
}

// NewAuthHandler 创建登录 handler。
func NewAuthHandler(svc *service.AuthService) *AuthHandler { return &AuthHandler{svc: svc} }

// Login POST /api/v1/auth/login：校验账号密码，返回 {token, user}。
//
//	@Summary 管理端登录
//	@Tags 认证
//	@Accept json
//	@Produce json
//	@Param body body loginRequest true "账号密码"
//	@Success 200 {object} resp.Envelope{data=api.loginResponse}
//	@Failure 401 {object} resp.Envelope "账号或密码错误"
//	@Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if !httputil.BindJSON(c, &req) {
		return
	}
	token, user, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, gin.H{"token": token, "user": user})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse 登录成功 data 字段结构（token + 管理员信息）。
type loginResponse struct {
	Token string        `json:"token"`
	User  model.AdminUser `json:"user"`
}
