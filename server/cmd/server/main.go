// server 入口：只负责加载配置与初始化日志；进程装配（MySQL/Redis/
// RabbitMQ/业务服务/HTTP）与生命周期管理全部在 internal/app 包内完成。
package main

import (
	"flag"
	"fmt"
	"os"

	"blog/server/internal/app"
	"blog/server/internal/config"
	"blog/server/internal/logger"
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

	// 装配并运行至停机，返回退出码；非零路径需手动 Sync（os.Exit 跳过 defer）
	if code := app.New(cfg, log).Run(); code != 0 {
		_ = log.Sync()
		os.Exit(code)
	}
}
