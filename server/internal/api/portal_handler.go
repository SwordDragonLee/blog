package api

import (
	"net/http"
	"strconv"
	"strings"

	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// PortalHandler 前台只读接口。
type PortalHandler struct {
	svc *service.PortalService
}

// NewPortalHandler 创建门户 handler。
func NewPortalHandler(svc *service.PortalService) *PortalHandler { return &PortalHandler{svc: svc} }

// ListArticles GET /portal/articles?page=&page_size=&tag=：已发布文章分页列表。
func (h *PortalHandler) ListArticles(c *gin.Context) {
	page, pageSize := pageParams(c)
	pd, err := h.svc.ListArticles(c.Request.Context(), page, pageSize, c.Query("tag"))
	if err != nil {
		failWith(c, err)
		return
	}
	ok(c, pd)
}

// GetArticle GET /portal/articles/:slug：文章详情（Markdown + 封面/配图元数据）。
func (h *PortalHandler) GetArticle(c *gin.Context) {
	detail, err := h.svc.GetArticle(c.Request.Context(), c.Param("slug"))
	if err != nil {
		failWith(c, err)
		return
	}
	ok(c, detail)
}

// Figure GET /portal/figures/:file：SVG 配图输出。
// :file 同时接受 "12.svg" 与 "12"；命中后以 image/svg+xml 输出并允许公开缓存。
func (h *PortalHandler) Figure(c *gin.Context) {
	id, okID := figureFileID(c.Param("file"))
	if !okID {
		fail(c, http.StatusBadRequest, "非法配图 id")
		return
	}
	asset, err := h.svc.GetFigure(c.Request.Context(), id)
	if err != nil {
		failWith(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "image/svg+xml", []byte(asset.SVGContent))
}

// figureFileID 解析配图文件名：剥掉 .svg 后缀（大小写不敏感）后须为纯数字 id。
func figureFileID(name string) (uint, bool) {
	if lower := strings.ToLower(name); strings.HasSuffix(lower, ".svg") {
		name = name[:len(name)-len(".svg")]
	}
	id, err := strconv.ParseUint(name, 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}
