package api

import (
	"net/http"
	"strconv"
	"strings"

	"blog/server/internal/httputil"
	"blog/server/internal/resp"
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// PortalHandler 前台接口：只读查询 + 文章点赞。
type PortalHandler struct {
	svc *service.PortalService
}

// NewPortalHandler 创建门户 handler。
func NewPortalHandler(svc *service.PortalService) *PortalHandler { return &PortalHandler{svc: svc} }

// ListArticles GET /portal/articles?page=&page_size=&tag=：已发布文章分页列表。
func (h *PortalHandler) ListArticles(c *gin.Context) {
	page, pageSize := httputil.PageParams(c)
	pd, err := h.svc.ListArticles(c.Request.Context(), page, pageSize, c.Query("tag"))
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, pd)
}

// GetArticle GET /portal/articles/:slug：文章详情（Markdown + 封面/配图元数据）。
func (h *PortalHandler) GetArticle(c *gin.Context) {
	detail, err := h.svc.GetArticle(c.Request.Context(), c.Param("slug"))
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, detail)
}

// RelatedArticles GET /portal/articles/:slug/related?limit=4：相关文章推荐。
// 语义向量为主（RAG 发布索引复用）、同仓库/最新发布兜底；RAG 未启用自动纯规则。
func (h *PortalHandler) RelatedArticles(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "4"))
	items, err := h.svc.GetRelated(c.Request.Context(), c.Param("slug"), limit)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, gin.H{"items": items})
}

// LikeArticle POST /portal/articles/:slug/like：点赞，按客户端 IP 去重；
// 重复点赞不报错，返回当前计数并置 duplicated=true。
func (h *PortalHandler) LikeArticle(c *gin.Context) {
	result, err := h.svc.LikeArticle(c.Request.Context(), c.Param("slug"), c.ClientIP())
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, result)
}

// Figure GET /portal/figures/:file：SVG 配图输出。
// :file 同时接受 "12.svg" 与 "12"；命中后以 image/svg+xml 输出并允许公开缓存。
func (h *PortalHandler) Figure(c *gin.Context) {
	id, okID := figureFileID(c.Param("file"))
	if !okID {
		resp.Fail(c, http.StatusBadRequest, "非法配图 id")
		return
	}
	asset, err := h.svc.GetFigure(c.Request.Context(), id)
	if err != nil {
		resp.FailWith(c, err)
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
