// Package router 装配 Gin 引擎：全局中间件与 admin / portal 两组路由。
package router

import (
	"blog/server/internal/api"
	"blog/server/internal/config"
	"blog/server/internal/middleware"
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Deps 路由装配依赖。
type Deps struct {
	Cfg      *config.Config
	Log      *zap.Logger
	Auth     *service.AuthService
	Tasks    *service.TaskService
	Articles *service.ArticleService
	Portal   *service.PortalService
	Rag      *service.RagService
}

// New 构建 gin 引擎与全部路由：
//
//	/api/v1/auth/login                     公开
//	/api/v1/tasks|articles|...             admin 组，JWT 保护
//	/api/v1/portal/*                       前台只读，公开
func New(d Deps) *gin.Engine {
	if d.Cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()
	r.Use(middleware.Recover(d.Log), middleware.ZapLogger(d.Log), middleware.CORS())

	// Prometheus 指标端点：供 Prometheus 抓取（blog_mq_* 等）。
	// 与业务同端口暴露；生产环境建议由网关/防火墙限制来源。
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	authHandler := api.NewAuthHandler(d.Auth)
	taskHandler := api.NewTaskHandler(d.Tasks)
	articleHandler := api.NewArticleHandler(d.Articles)
	portalHandler := api.NewPortalHandler(d.Portal)
	// RAG 问答：每 IP 每分钟 10 次（0 取默认值）
	ragHandler := api.NewRagHandler(d.Rag, 0, d.Log)

	v1 := r.Group("/api/v1")

	// 登录（公开）
	v1.POST("/auth/login", authHandler.Login)

	// 管理端（JWT 保护）
	admin := v1.Group("")
	admin.Use(middleware.JWTAuth(d.Auth))
	{
		admin.POST("/tasks", taskHandler.Create)
		admin.GET("/tasks", taskHandler.List)
		admin.GET("/tasks/:id", taskHandler.Get)
		admin.POST("/tasks/:id/retry", taskHandler.Retry)
		admin.POST("/tasks/:id/cancel", taskHandler.Cancel)
		admin.DELETE("/tasks/:id", taskHandler.Delete)

		admin.GET("/articles", articleHandler.List)
		admin.GET("/articles/:id", articleHandler.Get)
		admin.PUT("/articles/:id", articleHandler.Update)
		admin.POST("/articles/:id/publish", articleHandler.Publish)
		admin.POST("/articles/:id/offline", articleHandler.Offline)
		admin.POST("/articles/:id/regenerate-figures", articleHandler.RegenerateFigures)

		// profile：与登录共用 AuthHandler，走 admin JWT 组
		admin.GET("/auth/profile", authHandler.Profile)
		admin.PUT("/auth/profile", authHandler.UpdateProfile)

		// RAG 向量索引总览与检索测试（管理端，只读）
		admin.GET("/rag/index", ragHandler.Index)
		admin.GET("/rag/probe", ragHandler.Probe)
	}

	// 前台只读（公开）
	{
		v1.GET("/portal/articles", portalHandler.ListArticles)
		v1.GET("/portal/articles/:slug", portalHandler.GetArticle)
		v1.GET("/portal/figures/:file", portalHandler.Figure)
		// 唯一的前台写操作：点赞（按 IP 去重）
		v1.POST("/portal/articles/:slug/like", portalHandler.LikeArticle)
		// RAG 技术问答：SSE 流式接口，公开可访问（按 IP 限流）
		v1.POST("/portal/ask", ragHandler.Ask)
	}

	return r
}
