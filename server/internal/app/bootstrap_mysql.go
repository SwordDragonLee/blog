package app

import (
	"context"
	"fmt"

	"blog/server/internal/repo"

	"go.uber.org/zap"
)

// initMySQL 连接 MySQL 并执行迁移、种子管理员。
func (a *App) initMySQL(context.Context) error {
	db, err := repo.OpenMySQL(a.cfg.MySQL, a.cfg.Server.Mode)
	if err != nil {
		return fmt.Errorf("初始化 MySQL 失败: %w", err)
	}
	if err := repo.Migrate(db); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}
	if err := repo.SeedAdmin(db, a.cfg.Auth); err != nil {
		return fmt.Errorf("初始化管理员失败: %w", err)
	}
	a.db = db
	a.log.Info("MySQL 就绪", zap.String("database", a.cfg.MySQL.Database))
	return nil
}
