// Package repo 负责 MySQL / Redis 连接、建表迁移与管理员种子数据。
package repo

import (
	"context"
	"fmt"
	"time"

	"blog/server/internal/config"
	"blog/server/internal/model"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenMySQL 建立 GORM 连接（utf8mb4）。
func OpenMySQL(cfg config.MySQL, mode string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	logLevel := logger.Warn
	if mode == "debug" {
		logLevel = logger.Info
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// Migrate 自动迁移全部业务表。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.AdminUser{},
		&model.GenTask{},
		&model.RepoAnalysis{},
		&model.Article{},
		&model.SvgAsset{},
		&model.AskUnanswered{},
	)
}

// SeedAdmin 管理员表为空时按配置创建默认账号。
func SeedAdmin(db *gorm.DB, cfg config.Auth) error {
	var count int64
	if err := db.Model(&model.AdminUser{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return db.Create(&model.AdminUser{Username: cfg.AdminUser, PasswordHash: string(hash)}).Error
}

// OpenRedis 建立 Redis 客户端。
func OpenRedis(ctx context.Context, cfg config.Redis) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Addr, Password: cfg.Password, DB: cfg.DB})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("连接 Redis: %w", err)
	}
	return rdb, nil
}
