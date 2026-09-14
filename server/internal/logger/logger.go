// Package logger 提供 zap 日志器构建。
package logger

import "go.uber.org/zap"

// New 按运行模式构建 zap 日志器：release 用生产配置，其余用开发配置。
func New(mode string) (*zap.Logger, error) {
	if mode == "release" {
		return zap.NewProduction()
	}
	return zap.NewDevelopment()
}
