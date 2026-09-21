// Package app 负责进程装配与生命周期：按依赖顺序初始化基础设施
// （MySQL / Redis / RabbitMQ）、业务服务与 HTTP 服务，运行至收到
// SIGINT/SIGTERM 后按「HTTP → consumer → MQ」顺序优雅停机。
// cmd/server 只做配置加载，装配细节全部收敛在本包。
package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"blog/server/internal/config"
	"blog/server/internal/llm"
	"blog/server/internal/mq"
	"blog/server/internal/service"
	"blog/server/internal/task"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// 生命周期各阶段的超时预算。
const (
	httpShutdownTimeout  = 10 * time.Second // HTTP 优雅停机等待
	consumerStopTimeout  = 15 * time.Second // 等待 consumer 退出上限
	mqBootstrapTimeout   = 5 * time.Second  // 启动时 MQ 预连接超时
	redisConnectTimeout  = 5 * time.Second  // 启动时 Redis 连接超时
	progressWriteTimeout = 3 * time.Second  // 死信回调写 Redis 超时
	recoverTimeout       = 10 * time.Second // 启动时孤儿任务补投超时
)

// App 持有进程级组件，负责装配、启动与优雅停机。
type App struct {
	cfg *config.Config
	log *zap.Logger

	db        *gorm.DB
	rdb       *redis.Client
	broker    *mq.MQ
	publisher *mq.Publisher
	consumer  *mq.Consumer
	srv       *http.Server

	// 跨步骤共享的业务组件：initPipeline / initServices 构造，initMQ / initHTTP 使用
	llmClient  *llm.Client
	pipeline   *task.Pipeline
	ragSvc     *service.RagService
	authSvc    *service.AuthService
	taskSvc    *service.TaskService
	articleSvc *service.ArticleService
	portalSvc  *service.PortalService
}

// New 创建应用（不立即做任何 I/O，装配在 Run/bootstrap 中进行）。
func New(cfg *config.Config, log *zap.Logger) *App {
	return &App{cfg: cfg, log: log}
}

// Run 装配并运行直至收到停机信号，返回进程退出码
// （0 = 正常停机，1 = HTTP 服务异常退出或装配失败）。
func (a *App) Run() int {
	// 全局生命周期 ctx：收到 SIGINT/SIGTERM 后取消
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 开发自愈：debug 模式下自检「air 重编译但旧进程未被重启」的孤儿状态（见 dev_watchdog.go）
	if a.cfg.Server.Mode == "debug" {
		startStaleBinaryWatchdog(a.log)
	}

	if err := a.bootstrap(ctx); err != nil {
		a.log.Error("启动失败", zap.Error(err))
		return 1
	}

	// 后台 worker：MQ 消费 goroutine
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		a.consumer.Run(ctx)
	}()

	httpErr := make(chan error, 1)
	go func() {
		a.log.Info("HTTP 服务已启动",
			zap.Int("port", a.cfg.Server.Port), zap.String("mode", a.cfg.Server.Mode))
		httpErr <- a.srv.ListenAndServe()
	}()

	exitCode := 0
	select {
	case err := <-httpErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Error("HTTP 服务异常退出", zap.Error(err))
			exitCode = 1
		}
	case <-ctx.Done():
		a.log.Info("收到停机信号，开始优雅停机")
	}

	a.shutdown(ctx, stop, consumerDone)
	return exitCode
}

// shutdown 优雅停机：停 HTTP → 取消全局 ctx → 等 consumer 退出 → 关 MQ。
func (a *App) shutdown(ctx context.Context, stop context.CancelFunc, consumerDone <-chan struct{}) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	if err := a.srv.Shutdown(shutdownCtx); err != nil {
		a.log.Warn("HTTP 停机超时/出错", zap.Error(err))
	}
	stop() // 取消全局 ctx，consumer 退出循环

	select {
	case <-consumerDone:
	case <-time.After(consumerStopTimeout):
		a.log.Warn("等待 MQ consumer 退出超时，强制继续")
	}
	a.broker.Close()
	a.log.Info("服务已退出")
}
