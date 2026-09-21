package app

import (
	"context"

	"blog/server/internal/llm"
	"blog/server/internal/qdrant"
	"blog/server/internal/service"
	"blog/server/internal/task"
)

// initPipeline 组装 LLM 客户端、RAG 组件与生成任务流水线。
func (a *App) initPipeline(context.Context) error {
	a.llmClient = llm.NewClient(a.cfg.LLM, a.rdb, a.log)
	// RAG 技术问答组件：embedding 客户端 + Qdrant 向量库（rag.enabled=false 时服务自身不可用）
	embedder := llm.NewEmbedder(a.cfg.RAG, a.cfg.LLM, a.log)
	store := qdrant.New(a.cfg.Qdrant.BaseURL, a.cfg.Qdrant.Collection, a.log)
	// 问答面向公众、调用量大：单独配免费模型，不消耗主模型（生成套餐）额度
	chatClient := a.llmClient
	if qa := a.ragChatClient(); qa != nil {
		chatClient = qa
	}
	a.ragSvc = service.NewRagService(a.db, chatClient, embedder, store, a.cfg.RAG, a.log)
	a.pipeline = task.New(*a.cfg, a.db, a.rdb, a.llmClient, a.log)
	return nil
}

// ragChatClient 组装问答专用 chat 客户端（沿用 llm.Client 的双协议自动切换）；
// rag.chat_model 未配置时返回 nil，问答复用主模型客户端。
func (a *App) ragChatClient() *llm.Client {
	r := a.cfg.RAG
	if r.ChatModel == "" {
		return nil
	}
	c := a.cfg.LLM // 以主模型配置为底，逐项覆盖
	if r.ChatBaseURL != "" {
		c.BaseURL = r.ChatBaseURL
	}
	if r.ChatAPIKey != "" {
		c.APIKey = r.ChatAPIKey
	}
	c.Model = r.ChatModel
	c.MaxTokens = r.ChatMaxTokens // applyDefaults 已保证非零
	c.TimeoutSeconds = 120        // 单轮问答无需主模型的 600s 长超时
	return llm.NewClient(c, a.rdb, a.log)
}
