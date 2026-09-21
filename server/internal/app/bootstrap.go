package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// bootstrap 按依赖顺序执行初始化清单：任何一步失败即返回错误，
// 由 Run 统一记日志并以非零码退出。各步骤实现在同名 bootstrap_*.go。
func (a *App) bootstrap(ctx context.Context) error {
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"配置校验", a.initValidate},
		{"MySQL", a.initMySQL},
		{"Redis", a.initRedis},
		{"生成流水线", a.initPipeline},
		{"RabbitMQ", a.initMQ},
		{"业务服务", a.initServices},
		{"HTTP", a.initHTTP},
	}
	for _, s := range steps {
		if err := s.fn(ctx); err != nil {
			return fmt.Errorf("初始化 %s 失败: %w", s.name, err)
		}
	}
	return nil
}

// initValidate 校验启动必需的配置项。
func (a *App) initValidate(context.Context) error {
	// JWT 密钥是登录签发的前提，缺失时直接拒绝启动
	if strings.TrimSpace(a.cfg.Auth.JWTSecret) == "" {
		return errors.New("auth.jwt_secret 未配置")
	}
	return nil
}
