// Profile 接口：查询/更新当前登录管理员的信息（邮箱绑定）。
package api

import (
	"blog/server/internal/httputil"
	"blog/server/internal/middleware"
	"blog/server/internal/resp"

	"github.com/gin-gonic/gin"
)

// updateProfileRequest PUT /auth/profile 请求体。
type updateProfileRequest struct {
	Email string `json:"email"`
}

// Profile GET /api/v1/auth/profile：当前登录管理员的信息（用户名 + 邮箱）。
//
//	@Summary 查看当前管理员信息
//	@Tags 认证
//	@Security BearerAuth
//	@Produce json
//	@Success 200 {object} resp.Envelope{data=model.AdminUser}
//	@Router /auth/profile [get]
func (h *AuthHandler) Profile(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	user, err := h.svc.Profile(c.Request.Context(), uid)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, user)
}

// UpdateProfile PUT /api/v1/auth/profile：更新邮箱绑定。
//
//	@Summary 更新当前管理员邮箱
//	@Tags 认证
//	@Security BearerAuth
//	@Accept json
//	@Produce json
//	@Param body body updateProfileRequest true "邮箱"
//	@Success 200 {object} resp.Envelope{data=model.AdminUser}
//	@Router /auth/profile [put]
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	var req updateProfileRequest
	if !httputil.BindJSON(c, &req) {
		return
	}
	user, err := h.svc.UpdateProfile(c.Request.Context(), uid, req.Email)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, user)
}
