// Account 接口：当前登录管理员的用户名与密码自助维护（邮箱走 profile_handler）。
package api

import (
	"blog/server/internal/httputil"
	"blog/server/internal/middleware"
	"blog/server/internal/resp"

	"github.com/gin-gonic/gin"
)

// updateUsernameRequest PUT /auth/username 请求体。
type updateUsernameRequest struct {
	Username string `json:"username"`
}

// changePasswordRequest PUT /auth/password 请求体。
type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// UpdateUsername PUT /api/v1/auth/username：修改用户名，成功后重签 JWT。
//
//	@Summary 修改当前管理员用户名
//	@Tags 认证
//	@Security BearerAuth
//	@Accept json
//	@Produce json
//	@Param body body updateUsernameRequest true "新用户名"
//	@Success 200 {object} resp.Envelope{data=api.loginResponse} "返回新 token 与用户信息"
//	@Failure 400 {object} resp.Envelope "用户名不合法"
//	@Failure 409 {object} resp.Envelope "用户名已被占用"
//	@Router /auth/username [put]
func (h *AuthHandler) UpdateUsername(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	var req updateUsernameRequest
	if !httputil.BindJSON(c, &req) {
		return
	}
	token, user, err := h.svc.UpdateUsername(c.Request.Context(), uid, req.Username)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, gin.H{"token": token, "user": user})
}

// ChangePassword PUT /api/v1/auth/password：校验旧密码后修改密码。
// 前端约定：成功后清除登录态强制重新登录。
//
//	@Summary 修改当前管理员密码
//	@Tags 认证
//	@Security BearerAuth
//	@Accept json
//	@Produce json
//	@Param body body changePasswordRequest true "旧密码与新密码"
//	@Success 200 {object} resp.Envelope
//	@Failure 400 {object} resp.Envelope "旧密码错误或新密码不合法"
//	@Router /auth/password [put]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	var req changePasswordRequest
	if !httputil.BindJSON(c, &req) {
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), uid, req.OldPassword, req.NewPassword); err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, nil)
}
