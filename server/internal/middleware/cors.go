package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
