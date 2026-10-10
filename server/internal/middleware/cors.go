package middleware

import (
	"net"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// corsOrigins 允许跨域的前端开发来源。
var corsOrigins = map[string]bool{
	"http://localhost:5173": true, // Vite dev server（管理平台）
	"http://localhost:3000": true, // Next.js dev server（前台）
}

// originAllowed 白名单之外再放行来源为私网/loopback 地址的 http 请求：
// 手机/其他设备经局域网地址（如 http://192.168.0.124:3000）调试时 Origin 不是
// localhost，写死 IP 会随 DHCP 变动，按「私网或回环地址」判定。
// 端口不限定：Next dev 端口被占会自动递增（3000→3001→3002），
// 写死端口会在端口漂移后失效。生产环境走 nginx 同源不依赖本中间件，
// 且鉴权走 Authorization 头而非 Cookie，放宽来源无 CSRF 风险。
// 注意 loopback 必须显式放行：127.0.0.1 的 IsPrivate() 为 false。
func originAllowed(origin string) bool {
	if corsOrigins[origin] {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback())
}

// CORS 跨域中间件：白名单来源回显 Origin，OPTIONS 预检直接 204。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && originAllowed(origin) {
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
