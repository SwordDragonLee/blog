package app

import (
	"context"
	"fmt"

	"blog/server/internal/repo"

	"go.uber.org/zap"
)

// initRedis 连接 Redis（启动时带超时探测）。
func (a *App) initRedis(ctx context.Context) error {
	rdbCtx, cancel := context.WithTimeout(ctx, redisConnectTimeout)
	rdb, err := repo.OpenRedis(rdbCtx, a.cfg.Redis)
	cancel()
	if err != nil {
		return fmt.Errorf("初始化 Redis 失败: %w", err)
	}
	a.rdb = rdb
	a.log.Info("Redis 就绪", zap.String("addr", a.cfg.Redis.Addr))
	return nil
}
