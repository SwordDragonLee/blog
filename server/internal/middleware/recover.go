package middleware

import (
	"net/http"

	"blog/server/internal/resp"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recover panic 恢复中间件：记录日志并返回 JSON 信封 500。
func Recover(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic 已恢复",
					zap.Any("panic", r),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path))
				if c.Writer.Written() {
					c.AbortWithStatus(http.StatusInternalServerError)
					return
				}
				resp.Abort(c, http.StatusInternalServerError, "服务器内部错误")
			}
		}()
		c.Next()
	}
}
