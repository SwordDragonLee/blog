// Package api 实现 HTTP 层：路由、中间件（CORS / 日志 / recover / JWT）
// 与 admin、portal 两组 handler，统一响应信封 {code, message, data}。
package api

import (
	"errors"
	"net/http"
	"strconv"

	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// envelope 统一响应结构。成功 code=0、message="ok"；失败 code 为 HTTP 状态码。
type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// ok 输出成功响应。
func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{Code: 0, Message: "ok", Data: data})
}

// fail 输出失败响应（code 与 HTTP 状态码一致）。
func fail(c *gin.Context, status int, message string) {
	c.JSON(status, envelope{Code: status, Message: message, Data: nil})
}

// failWith 将 service 层错误映射为 HTTP 状态码输出。
func failWith(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalid):
		fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrUnauthorized):
		fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, service.ErrNotFound):
		fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrConflict):
		fail(c, http.StatusConflict, err.Error())
	default:
		fail(c, http.StatusInternalServerError, err.Error())
	}
}

// bindJSON 解析 JSON 请求体，失败时输出 400 并返回 false。
func bindJSON(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		fail(c, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return false
	}
	return true
}

// pathUint 解析路径参数为正整数 id，失败时输出 400 并返回 false。
func pathUint(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "路径参数 "+name+" 需为正整数")
		return 0, false
	}
	return uint(id), true
}

// pageParams 解析分页参数（?page=&page_size=）。
func pageParams(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.Query("page"))
	pageSize, _ = strconv.Atoi(c.Query("page_size"))
	return service.NormalizePage(page, pageSize)
}
