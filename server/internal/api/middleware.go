package api

import (
	"net/http"
	"strings"
	"time"

	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// gin context 中存放 JWT claims 的 key。
const (
	CtxUserID   = "ctx_user_id"
	CtxUsername = "ctx_username"
)

// corsOrigins 允许跨域的前端开发来源。
var corsOrigins = map[string]bool{
	"http://localhost:5173": true, // Vite dev server（管理平台）
	"http://localhost:3000": true, // Next.js dev server（前台）
}

// CORS 跨域中间件：白名单来源回显 Origin，OPTIONS 预检直接 204。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && corsOrigins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// ZapLogger 请求日志中间件：方法、路径、状态码、耗时、来源 IP。
func ZapLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		}
		if query != "" {
			fields = append(fields, zap.String("query", query))
		}
		if errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String(); errMsg != "" {
			fields = append(fields, zap.String("errors", errMsg))
			log.Error("http request", fields...)
			return
		}
		log.Info("http request", fields...)
	}
}

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
				c.AbortWithStatusJSON(http.StatusInternalServerError,
					envelope{Code: http.StatusInternalServerError, Message: "服务器内部错误", Data: nil})
			}
		}()
		c.Next()
	}
}

// JWTAuth Bearer 令牌校验中间件：解析失败一律 401（前端据此跳转登录）。
// 通过后把 user id / username 写入 gin context。
func JWTAuth(auth *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			abort401(c, "缺少认证令牌")
			return
		}
		claims, err := auth.ParseToken(token)
		if err != nil {
			abort401(c, "登录已过期或令牌无效")
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

// abort401 输出 401 信封并中断请求链。
func abort401(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized,
		envelope{Code: http.StatusUnauthorized, Message: message, Data: nil})
}
