package api

import (
	"blog/server/internal/config"
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
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
	r.Use(Recover(d.Log), ZapLogger(d.Log), CORS())

	authHandler := NewAuthHandler(d.Auth)
	taskHandler := NewTaskHandler(d.Tasks)
	articleHandler := NewArticleHandler(d.Articles)
	portalHandler := NewPortalHandler(d.Portal)

	v1 := r.Group("/api/v1")

	// 登录（公开）
	v1.POST("/auth/login", authHandler.Login)

	// 管理端（JWT 保护）
	admin := v1.Group("")
	admin.Use(JWTAuth(d.Auth))
	{
		admin.POST("/tasks", taskHandler.Create)
		admin.GET("/tasks", taskHandler.List)
		admin.GET("/tasks/:id", taskHandler.Get)
		admin.POST("/tasks/:id/retry", taskHandler.Retry)

		admin.GET("/articles", articleHandler.List)
		admin.GET("/articles/:id", articleHandler.Get)
		admin.PUT("/articles/:id", articleHandler.Update)
		admin.POST("/articles/:id/publish", articleHandler.Publish)
		admin.POST("/articles/:id/offline", articleHandler.Offline)
		admin.POST("/articles/:id/regenerate-figures", articleHandler.RegenerateFigures)
	}

	// 前台只读（公开）
	{
		v1.GET("/portal/articles", portalHandler.ListArticles)
		v1.GET("/portal/articles/:slug", portalHandler.GetArticle)
		v1.GET("/portal/figures/:file", portalHandler.Figure)
	}

	return r
}
