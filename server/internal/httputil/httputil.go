// Package httputil 提供 handler 层的请求解析辅助：JSON 请求体、路径参数、分页参数，
// 解析失败时按统一信封输出 400。
package httputil

import (
	"net/http"
	"strconv"

	"blog/server/internal/resp"
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// BindJSON 解析 JSON 请求体，失败时输出 400 并返回 false。
func BindJSON(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		resp.Fail(c, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return false
	}
	return true
}

// PathUint 解析路径参数为正整数 id，失败时输出 400 并返回 false。
func PathUint(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		resp.Fail(c, http.StatusBadRequest, "路径参数 "+name+" 需为正整数")
		return 0, false
	}
	return uint(id), true
}

// PageParams 解析分页参数（?page=&page_size=），归一化后返回。
func PageParams(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.Query("page"))
	pageSize, _ = strconv.Atoi(c.Query("page_size"))
	return service.NormalizePage(page, pageSize)
}
