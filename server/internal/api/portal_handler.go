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

// ListArticles GET /api/v1/portal/articles?page=&page_size=&tag=：已发布文章分页列表。
//
//	@Summary 文章分页列表（前台）
//	@Tags 门户
//	@Produce json
//	@Param page query int false "页码，默认 1"
//	@Param page_size query int false "每页条数，默认 10，上限 100"
//	@Param tag query string false "按标签过滤"
//	@Success 200 {object} resp.Envelope{data=service.PageData{items=[]model.Article}}
//	@Router /portal/articles [get]
func (h *PortalHandler) ListArticles(c *gin.Context) {
	page, pageSize := httputil.PageParams(c)
	pd, err := h.svc.ListArticles(c.Request.Context(), page, pageSize, c.Query("tag"))
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, pd)
}

// GetArticle GET /api/v1/portal/articles/:slug：文章详情（Markdown + 封面/配图元数据）。
//
//	@Summary 文章详情（前台）
//	@Tags 门户
//	@Produce json
//	@Param slug path string true "文章 slug"
//	@Success 200 {object} resp.Envelope{data=service.PortalArticleDetail}
//	@Router /portal/articles/{slug} [get]
func (h *PortalHandler) GetArticle(c *gin.Context) {
	detail, err := h.svc.GetArticle(c.Request.Context(), c.Param("slug"))
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, detail)
}

// RelatedArticles GET /api/v1/portal/articles/:slug/related?limit=4：相关文章推荐。
// 语义向量为主（RAG 发布索引复用）、同仓库/最新发布兜底；RAG 未启用自动纯规则。
//
//	@Summary 相关文章推荐
//	@Tags 门户
//	@Produce json
//	@Param slug path string true "文章 slug"
//	@Param limit query int false "返回条数，默认 4"
//	@Success 200 {object} resp.Envelope{data=object{items=[]service.PortalRelatedArticle}}
//	@Router /portal/articles/{slug}/related [get]
func (h *PortalHandler) RelatedArticles(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "4"))
	items, err := h.svc.GetRelated(c.Request.Context(), c.Param("slug"), limit)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, gin.H{"items": items})
}

// LikeArticle POST /api/v1/portal/articles/:slug/like：点赞，按客户端 IP 去重；
// 重复点赞不报错，返回当前计数并置 duplicated=true。
//
//	@Summary 文章点赞
//	@Tags 门户
//	@Produce json
//	@Param slug path string true "文章 slug"
//	@Success 200 {object} resp.Envelope{data=service.PortalLikeResult}
//	@Router /portal/articles/{slug}/like [post]
func (h *PortalHandler) LikeArticle(c *gin.Context) {
	result, err := h.svc.LikeArticle(c.Request.Context(), c.Param("slug"), c.ClientIP())
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, result)
}

// Figure GET /api/v1/portal/figures/:file：SVG 配图输出。
// :file 同时接受 "12.svg" 与 "12"；命中后以 image/svg+xml 输出并允许公开缓存。
//
//	@Summary SVG 配图
//	@Description 前台 Markdown 内以 /api/v1/portal/figures/xx.svg 引用；非 JSON，直接输出 SVG。
//	@Tags 门户
//	@Produce image/svg+xml
//	@Param file path string true "配图文件名，如 12.svg 或 12"
//	@Success 200 {string} string "image/svg+xml"
//	@Failure 400 {object} resp.Envelope "非法配图 id"
//	@Router /portal/figures/{file} [get]
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
