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

// List GET /api/v1/articles?status=&page=&page_size=：文章分页列表。
//
//	@Summary 文章分页列表（管理端）
//	@Tags 文章
//	@Security BearerAuth
//	@Produce json
//	@Param status query string false "状态过滤：draft / published，可省略"
//	@Param page query int false "页码，默认 1"
//	@Param page_size query int false "每页条数，默认 10，上限 100"
//	@Success 200 {object} resp.Envelope{data=service.PageData{items=[]model.Article}}
//	@Router /articles [get]
func (h *ArticleHandler) List(c *gin.Context) {
	page, pageSize := httputil.PageParams(c)
	pd, err := h.svc.List(c.Request.Context(), c.Query("status"), page, pageSize)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, pd)
}

// Get GET /api/v1/articles/:id：文章详情（含配图元数据）。
//
//	@Summary 文章详情
//	@Tags 文章
//	@Security BearerAuth
//	@Produce json
//	@Param id path int true "文章 ID"
//	@Success 200 {object} resp.Envelope{data=service.ArticleDetail}
//	@Router /articles/{id} [get]
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

// Update PUT /api/v1/articles/:id：编辑标题/摘要/正文/标签。
// 已发布文章保存后回到 draft，需重新发版。
//
//	@Summary 编辑文章
//	@Description 已发布文章保存后自动退回 draft，需重新发版。
//	@Tags 文章
//	@Security BearerAuth
//	@Accept json
//	@Produce json
//	@Param id path int true "文章 ID"
//	@Param body body updateArticleRequest true "标题/摘要/正文/标签"
//	@Success 200 {object} resp.Envelope{data=model.Article}
//	@Router /articles/{id} [put]
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

// Publish POST /api/v1/articles/:id/publish：发版（draft → published）。
//
//	@Summary 发版文章
//	@Description draft → published；同时触发 Redis 前台缓存清理与 RAG 向量化（异步）。
//	@Tags 文章
//	@Security BearerAuth
//	@Produce json
//	@Param id path int true "文章 ID"
//	@Success 200 {object} resp.Envelope{data=model.Article}
//	@Router /articles/{id}/publish [post]
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

// Offline POST /api/v1/articles/:id/offline：下线（published → draft）。
//
//	@Summary 下线文章
//	@Description published → draft；同时清理 Redis 前台缓存并删除该篇 RAG 向量。
//	@Tags 文章
//	@Security BearerAuth
//	@Produce json
//	@Param id path int true "文章 ID"
//	@Success 200 {object} resp.Envelope{data=model.Article}
//	@Router /articles/{id}/offline [post]
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

// RegenerateFigures POST /api/v1/articles/:id/regenerate-figures：重新生成该篇配图。
// 涉及 LLM 调用，耗时较长，前端需放宽请求超时。
//
//	@Summary 重新生成配图
//	@Tags 文章
//	@Security BearerAuth
//	@Produce json
//	@Param id path int true "文章 ID"
//	@Success 200 {object} resp.Envelope{data=service.ArticleDetail}
//	@Router /articles/{id}/regenerate-figures [post]
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
