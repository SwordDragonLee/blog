package app

import (
	"context"

	"blog/server/internal/mailer"
	"blog/server/internal/service"

	"go.uber.org/zap"
)

// initServices 构造业务服务并做启动期补偿（孤儿任务补投）。
func (a *App) initServices(context.Context) error {
	mail := mailer.NewMailer(a.db, a.cfg.Email, a.log)
	a.authSvc = service.NewAuthService(a.db, a.cfg.Auth, a.log)
	a.taskSvc = service.NewTaskService(a.db, a.rdb, a.publisher, a.log)
	a.articleSvc = service.NewArticleService(a.db, a.rdb, a.llmClient, mail, a.ragSvc, a.log)
	a.portalSvc = service.NewPortalService(a.db, a.rdb, a.ragSvc, a.log)
	// 运行中任务可被主动取消：service 层回调 consumer 的按任务取消
	a.taskSvc.SetCancelFunc(a.consumer.CancelTask)
	// 删除任务时同步清理文章在 Qdrant 的残留向量
	a.taskSvc.SetRagCleaner(a.ragSvc)

	// 孤儿任务补偿：补投「已落库但从未成功进入 MQ」的任务消息
	// （Create/Retry 在落库与投递之间崩溃所遗留，判据为 queued_at IS NULL）
	recoverCtx, cancel := context.WithTimeout(context.Background(), recoverTimeout)
	_, err := a.taskSvc.RecoverUnqueued(recoverCtx)
	cancel()
	if err != nil {
		a.log.Warn("孤儿任务扫描失败（下次启动重试）", zap.Error(err))
	}
	return nil
}
