package api

import (
	"blog/server/internal/httputil"
	"blog/server/internal/middleware"
	"blog/server/internal/resp"
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// ArticleHandler 文章管理接口（admin）。
type ArticleHandler struct {
	svc *service.ArticleService
}

// NewArticleHandler 创建文章 handler。
func NewArticleHandler(svc *service.ArticleService) *ArticleHandler { return &ArticleHandler{svc: svc} }

// List GET /articles?status=&page=&page_size=：文章分页列表。
func (h *ArticleHandler) List(c *gin.Context) {
	page, pageSize := httputil.PageParams(c)
	pd, err := h.svc.List(c.Request.Context(), c.Query("status"), page, pageSize)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, pd)
}

// Get GET /articles/:id：文章详情（含配图元数据）。
func (h *ArticleHandler) Get(c *gin.Context) {
	id, okID := httputil.PathUint(c, "id")
	if !okID {
		return
	}
	detail, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, detail)
}

// Update PUT /articles/:id：编辑标题/摘要/正文/标签。
// 已发布文章保存后回到 draft，需重新发版。
func (h *ArticleHandler) Update(c *gin.Context) {
	id, okID := httputil.PathUint(c, "id")
	if !okID {
		return
	}
	var req updateArticleRequest
	if !httputil.BindJSON(c, &req) {
		return
	}
	art, err := h.svc.Update(c.Request.Context(), id, req.Title, req.Summary, req.ContentMD, req.Tags)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, art)
}

// Publish POST /articles/:id/publish：发版（draft → published）。
func (h *ArticleHandler) Publish(c *gin.Context) {
	id, okID := httputil.PathUint(c, "id")
	if !okID {
		return
	}
	art, err := h.svc.Publish(c.Request.Context(), id, c.GetUint(middleware.CtxUserID))
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, art)
}

// Offline POST /articles/:id/offline：下线（published → draft）。
func (h *ArticleHandler) Offline(c *gin.Context) {
	id, okID := httputil.PathUint(c, "id")
	if !okID {
		return
	}
	art, err := h.svc.Offline(c.Request.Context(), id)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, art)
}

// RegenerateFigures POST /articles/:id/regenerate-figures：重新生成该篇配图。
// 涉及 LLM 调用，耗时较长，前端需放宽请求超时。
func (h *ArticleHandler) RegenerateFigures(c *gin.Context) {
	id, okID := httputil.PathUint(c, "id")
	if !okID {
		return
	}
	detail, err := h.svc.RegenerateFigures(c.Request.Context(), id)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, detail)
}

type updateArticleRequest struct {
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	ContentMD string   `json:"content_md"`
	Tags      []string `json:"tags"`
}
