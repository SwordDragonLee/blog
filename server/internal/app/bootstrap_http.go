package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"blog/server/internal/router"
)

// initHTTP 组装路由并创建 HTTP Server（不启动监听，Run 中 ListenAndServe）。
func (a *App) initHTTP(context.Context) error {
	a.srv = &http.Server{
		Addr: fmt.Sprintf(":%d", a.cfg.Server.Port),
		Handler: router.New(router.Deps{
			Cfg:      a.cfg,
			Log:      a.log,
			Auth:     a.authSvc,
			Tasks:    a.taskSvc,
			Articles: a.articleSvc,
			Portal:   a.portalSvc,
			Rag:      a.ragSvc,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return nil
}
