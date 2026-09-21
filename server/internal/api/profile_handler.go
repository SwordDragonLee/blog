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

// Profile GET /auth/profile：当前登录管理员的信息（用户名 + 邮箱）。
func (h *AuthHandler) Profile(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	user, err := h.svc.Profile(c.Request.Context(), uid)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, user)
}

// UpdateProfile PUT /auth/profile：更新邮箱绑定。
// UpdateProfile PUT /auth/profile：更新邮箱绑定。
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
