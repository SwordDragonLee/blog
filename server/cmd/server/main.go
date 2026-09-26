// server 入口：只负责加载配置与初始化日志；进程装配（MySQL/Redis/
// RabbitMQ/业务服务/HTTP）与生命周期管理全部在 internal/app 包内完成。
//
//	@title AI 博客平台 API
//	@version 1.0
//	@description AI 博客生成平台后端：管理端（任务/文章/RAG 向量索引）+ 前台门户（文章浏览/RAG 问答）。
//	@description 统一响应结构 resp.Envelope{code, message, data}，code=0 表示成功；业务错误 code 为 HTTP 状态码。
//	@description /metrics 为 Prometheus 抓取端点，不在 /api/v1 之下，本文档未收录。
//	@BasePath /api/v1
//
//	@SecurityDefinitions.apikey BearerAuth
//	@in header
//	@name Authorization
//	@description 管理端接口需在请求头携带登录接口返回的 JWT：`Authorization: Bearer <token>`。
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
