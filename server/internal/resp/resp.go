// Package resp 提供统一的 HTTP JSON 响应信封 {code, message, data}：
// 成功 code=0、message="ok"；失败 code 与 HTTP 状态码一致，
// 并负责把 service 层哨兵错误映射为对应 HTTP 状态码。
package resp

import (
	"errors"
	"net/http"

	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// Envelope 统一响应结构。
type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// OK 输出成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Code: 0, Message: "ok", Data: data})
}

// Fail 输出失败响应（code 与 HTTP 状态码一致）。
func Fail(c *gin.Context, status int, message string) {
	c.JSON(status, Envelope{Code: status, Message: message, Data: nil})
}

// Abort 输出失败响应并中断后续 handler 链（中间件使用）。
func Abort(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, Envelope{Code: status, Message: message, Data: nil})
}

// FailWith 将 service 层哨兵错误映射为 HTTP 状态码输出。
func FailWith(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalid):
		Fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrUnauthorized):
		Fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, service.ErrNotFound):
		Fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrConflict):
		Fail(c, http.StatusConflict, err.Error())
	default:
		Fail(c, http.StatusInternalServerError, err.Error())
	}
}
