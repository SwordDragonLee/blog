// server 入口：装配配置、日志、MySQL、Redis、LLM 客户端、任务流水线、
// RabbitMQ（Publisher + Consumer）与 Gin HTTP 服务，单二进制同时提供
// API 与后台 worker，支持 SIGINT/SIGTERM 优雅停机。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"blog/server/internal/api"
	"blog/server/internal/config"
	"blog/server/internal/llm"
	"blog/server/internal/logger"
	"blog/server/internal/model"
	"blog/server/internal/mq"
	"blog/server/internal/repo"
	"blog/server/internal/service"
	"blog/server/internal/task"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	httpShutdownTimeout  = 10 * time.Second // HTTP 优雅停机等待
	consumerStopTimeout  = 15 * time.Second // 等待 consumer 退出上限
	mqBootstrapTimeout   = 5 * time.Second  // 启动时 MQ 预连接超时
	redisConnectTimeout  = 5 * time.Second  // 启动时 Redis 连接超时
	progressWriteTimeout = 3 * time.Second  // 死信回调写 Redis 超时
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		os.Exit(1)
	}
	log, err := logger.New(cfg.Server.Mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "初始化日志失败:", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	if strings.TrimSpace(cfg.Auth.JWTSecret) == "" {
		log.Fatal("auth.jwt_secret 未配置，拒绝启动")
	}

	// MySQL：连接 + 建表 + 管理员种子
	db, err := repo.OpenMySQL(cfg.MySQL, cfg.Server.Mode)
	if err != nil {
		log.Fatal("初始化 MySQL 失败", zap.Error(err))
	}
	if err := repo.Migrate(db); err != nil {
		log.Fatal("数据库迁移失败", zap.Error(err))
	}
	if err := repo.SeedAdmin(db, cfg.Auth); err != nil {
		log.Fatal("初始化管理员失败", zap.Error(err))
	}
	log.Info("MySQL 就绪", zap.String("database", cfg.MySQL.Database))

	// 全局生命周期 ctx：收到 SIGINT/SIGTERM 后取消
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Redis
	rdbCtx, rdbCancel := context.WithTimeout(ctx, redisConnectTimeout)
	rdb, err := repo.OpenRedis(rdbCtx, cfg.Redis)
	rdbCancel()
	if err != nil {
		log.Fatal("初始化 Redis 失败", zap.Error(err))
	}
	log.Info("Redis 就绪", zap.String("addr", cfg.Redis.Addr))

	// LLM 客户端 + 任务流水线
	llmClient := llm.NewClient(cfg.LLM, rdb, log)
	pipeline := task.New(*cfg, db, rdb, llmClient, log)

	// RabbitMQ：连接管理 + 发布者 + 消费者
	broker := mq.New(cfg.RabbitMQ.URL, log)
	mqCtx, mqCancel := context.WithTimeout(ctx, mqBootstrapTimeout)
	_, err = broker.Connect(mqCtx)
	mqCancel()
	if err != nil {
		// 启动时不因 MQ 不可用而退出：Publisher / Consumer 均会按需重连
		log.Warn("RabbitMQ 暂不可用，将在使用时自动重连", zap.Error(err))
	}
	publisher := mq.NewPublisher(broker)
	consumer := mq.NewConsumer(broker, cfg.Task, log, pipeline.Handle)
	consumer.OnDead = newOnDead(db, rdb, log)
	log.Info("RabbitMQ 已初始化", zap.Int("concurrency", max(cfg.Task.Concurrency, 1)))

	// 业务服务 + 路由
	authSvc := service.NewAuthService(db, cfg.Auth, log)
	taskSvc := service.NewTaskService(db, rdb, publisher, log)
	articleSvc := service.NewArticleService(db, rdb, llmClient, log)
	portalSvc := service.NewPortalService(db, rdb, log)

	router := api.New(api.Deps{
		Cfg:      cfg,
		Log:      log,
		Auth:     authSvc,
		Tasks:    taskSvc,
		Articles: articleSvc,
		Portal:   portalSvc,
	})
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 后台 worker：MQ 消费 goroutine
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		consumer.Run(ctx)
	}()

	httpErr := make(chan error, 1)
	go func() {
		log.Info("HTTP 服务已启动", zap.Int("port", cfg.Server.Port), zap.String("mode", cfg.Server.Mode))
		httpErr <- srv.ListenAndServe()
	}()

	exitCode := 0
	select {
	case err := <-httpErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("HTTP 服务异常退出", zap.Error(err))
			exitCode = 1
		}
	case <-ctx.Done():
		log.Info("收到停机信号，开始优雅停机")
	}

	// 优雅停机：停 HTTP → 停 consumer → 关 MQ
	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("HTTP 停机超时/出错", zap.Error(err))
	}
	stop() // 取消全局 ctx，consumer 退出循环

	select {
	case <-consumerDone:
	case <-time.After(consumerStopTimeout):
		log.Warn("等待 MQ consumer 退出超时，强制继续")
	}
	broker.Close()
	log.Info("服务已退出")
	if exitCode != 0 {
		_ = log.Sync()
		os.Exit(exitCode)
	}
}

// newOnDead 消息重试超限进入死信队列前的回调：把任务标记为 failed，
// 并同步 Redis 进度供管理端轮询展示。
func newOnDead(db *gorm.DB, rdb *redis.Client, log *zap.Logger) func(ctx context.Context, taskID uint) {
	return func(ctx context.Context, taskID uint) {
		errMsg := "任务多次重试仍失败，已进入死信队列"
		if err := db.WithContext(ctx).Model(&model.GenTask{}).
			Where("id = ?", taskID).
			Updates(map[string]any{
				"status":  model.TaskStatusFailed,
				"step":    "失败",
				"message": "任务失败",
				"error":   errMsg,
			}).Error; err != nil {
			log.Error("标记任务失败状态出错", zap.Uint("task_id", taskID), zap.Error(err))
		}
		if rdb != nil {
			pctx, cancel := context.WithTimeout(context.Background(), progressWriteTimeout)
			defer cancel()
			if err := rdb.HSet(pctx, task.ProgressKey(taskID), map[string]any{
				"step":       "失败",
				"percent":    0,
				"message":    errMsg,
				"updated_at": time.Now().Format(time.RFC3339),
			}).Err(); err != nil {
				log.Warn("写入任务进度失败", zap.Uint("task_id", taskID), zap.Error(err))
			}
		}
		log.Error("任务进入死信队列", zap.Uint("task_id", taskID))
	}
}

// maxInt 返回两者中较大值。
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
