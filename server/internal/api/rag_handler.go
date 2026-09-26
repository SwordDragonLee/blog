package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"blog/server/internal/httputil"
	"blog/server/internal/resp"
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RagHandler RAG 技术问答（前台公开接口，SSE 流式）。
type RagHandler struct {
	svc     *service.RagService
	limiter *rateLimiter
	log     *zap.Logger
}

// NewRagHandler 创建问答 handler；limit 为每 IP 每分钟提问上限。
func NewRagHandler(svc *service.RagService, limit int, log *zap.Logger) *RagHandler {
	if limit <= 0 {
		limit = 10
	}
	return &RagHandler{svc: svc, limiter: newRateLimiter(limit), log: log}
}

// askRequest POST /portal/ask 请求体。
type askRequest struct {
	Question string         `json:"question"`
	History  []service.Turn `json:"history"`
}

// Ask POST /api/v1/portal/ask：SSE 流式回答。
// 事件序列：delta（增量文本）* N → citations（引用来源列表）→ done；
// 服务端出错时发送 error 事件（JSON 文本）后结束。
//
//	@Summary RAG 技术问答（SSE 流式）
//	@Description 向量检索 top_k 片段 + LLM 流式生成；按 IP 限流（每分钟 10 次）。
//	@Description 请求体：question 必填；history 为可选多轮上下文。
//	@Description 响应为 text/event-stream，事件序列 delta* → citations → done，出错发 error 事件。
//	@Tags RAG 问答
//	@Accept json
//	@Produce text/event-stream
//	@Param body body askRequest true "问题与可选历史"
//	@Success 200 {string} string "text/event-stream（SSE 事件流，非 JSON）"
//	@Failure 400 {object} resp.Envelope "参数错误"
//	@Failure 429 {object} resp.Envelope "提问太频繁"
//	@Failure 503 {object} resp.Envelope "问答功能未启用"
//	@Router /portal/ask [post]
func (h *RagHandler) Ask(c *gin.Context) {
	if !h.svc.Enabled() {
		resp.Fail(c, http.StatusServiceUnavailable, "问答功能未启用")
		return
	}
	var req askRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Question == "" {
		resp.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	ip := c.ClientIP()
	if !h.limiter.allow(ip) {
		resp.Fail(c, http.StatusTooManyRequests, "提问太频繁，请稍后再试")
		return
	}

	// SSE 响应头：此后不再走 resp 统一封装，错误以 error 事件下发
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // Nginx 反代时不缓冲
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		resp.Fail(c, http.StatusInternalServerError, "当前环境不支持流式响应")
		return
	}

	writeEvent := func(event string, data any) {
		payload, _ := json.Marshal(data)
		_, _ = c.Writer.WriteString("event: " + event + "\ndata: " + string(payload) + "\n\n")
		flusher.Flush()
	}
	onDelta := func(text string) { writeEvent("delta", map[string]string{"text": text}) }

	citations, err := h.svc.Ask(c.Request.Context(), req.Question, req.History, onDelta)
	if err != nil {
		h.log.Warn("问答失败", zap.String("ip", ip), zap.Error(err))
		writeEvent("error", map[string]string{"message": "回答生成失败，请稍后重试"})
		return
	}
	writeEvent("citations", citations)
	writeEvent("done", map[string]any{})
}

// Rebuild POST /api/v1/rag/index/rebuild：全量重建已发布文章向量（管理端）。
// 串行逐篇重新切块向量化覆盖写入，返回各篇成功/失败汇总；断连不中断（WithoutCancel）。
// 用于切块算法或 embedding 模型变更后刷新存量向量，避免逐篇重新发版。
//
//	@Summary 全量重建向量索引
//	@Tags RAG 管理
//	@Security BearerAuth
//	@Produce json
//	@Success 200 {object} resp.Envelope{data=service.RebuildReport}
//	@Failure 503 {object} resp.Envelope "问答功能未启用"
//	@Router /rag/index/rebuild [post]
func (h *RagHandler) Rebuild(c *gin.Context) {
	if !h.svc.Enabled() {
		resp.Fail(c, http.StatusServiceUnavailable, "问答功能未启用")
		return
	}
	ctx := context.WithoutCancel(c.Request.Context())
	report, err := h.svc.RebuildAll(ctx)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, report)
}

// Index GET /api/v1/rag/index：向量索引总览（管理端）。
//
//	@Summary 向量索引总览
//	@Description 滚动全量向量点、按文章分组统计块数；用于核对漏索引/残留向量。
//	@Tags RAG 管理
//	@Security BearerAuth
//	@Produce json
//	@Success 200 {object} resp.Envelope{data=service.RagIndexOverview}
//	@Failure 503 {object} resp.Envelope "问答功能未启用"
//	@Router /rag/index [get]
func (h *RagHandler) Index(c *gin.Context) {
	if !h.svc.Enabled() {
		resp.Fail(c, http.StatusServiceUnavailable, "问答功能未启用")
		return
	}
	ov, err := h.svc.IndexOverview(c.Request.Context())
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, ov)
}

// Probe GET /api/v1/rag/probe?q=&threshold=&limit=：检索测试（管理端）。
// 只返回原始命中与得分分布、不生成回答，用于观测检索质量、校准 score_threshold。
//
//	@Summary 检索测试
//	@Tags RAG 管理
//	@Security BearerAuth
//	@Produce json
//	@Param q query string true "测试问题"
//	@Param threshold query number false "相似度阈值 0~1，缺省取 rag.score_threshold 配置"
//	@Param limit query int false "返回条数 1~20，默认 10"
//	@Success 200 {object} resp.Envelope{data=[]service.ProbeHit}
//	@Failure 400 {object} resp.Envelope "缺少参数 q / 参数非法"
//	@Router /rag/probe [get]
func (h *RagHandler) Probe(c *gin.Context) {
	if !h.svc.Enabled() {
		resp.Fail(c, http.StatusServiceUnavailable, "问答功能未启用")
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		resp.Fail(c, http.StatusBadRequest, "缺少参数 q")
		return
	}
	threshold := h.svc.ScoreThreshold()
	if v := c.Query("threshold"); v != "" {
		f, err := strconv.ParseFloat(v, 32)
		if err != nil || f < 0 || f > 1 {
			resp.Fail(c, http.StatusBadRequest, "threshold 需为 0~1 的数值")
			return
		}
		threshold = float32(f)
	}
	limit := 10
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 20 {
			resp.Fail(c, http.StatusBadRequest, "limit 需为 1~20 的整数")
			return
		}
		limit = n
	}
	hits, err := h.svc.Probe(c.Request.Context(), q, limit, threshold)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, hits)
}

// ListUnanswered GET /api/v1/rag/unanswered?keyword=&start=&end=：无命中问题列表（管理端，选题回流）。
// keyword 模糊匹配问题原文与归一化键；start/end 为 YYYY-MM-DD，按最近提问时间过滤（含 end 当天），均可省略。
//
//	@Summary 无命中问题列表
//	@Description RAG 问答无命中时落库的问题（归一化去重、计次），用于选题回流。
//	@Tags RAG 管理
//	@Security BearerAuth
//	@Produce json
//	@Param keyword query string false "模糊匹配问题原文与归一化键"
//	@Param start query string false "起始日期 YYYY-MM-DD（含）"
//	@Param end query string false "结束日期 YYYY-MM-DD（含当天）"
//	@Param page query int false "页码，默认 1"
//	@Param page_size query int false "每页条数，默认 10，上限 100"
//	@Success 200 {object} resp.Envelope{data=service.PageData{items=[]model.AskUnanswered}}
//	@Router /rag/unanswered [get]
func (h *RagHandler) ListUnanswered(c *gin.Context) {
	page, pageSize := httputil.PageParams(c)
	keyword := strings.TrimSpace(c.Query("keyword"))
	var start, end *time.Time
	if v := c.Query("start"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, time.Local)
		if err != nil {
			resp.Fail(c, http.StatusBadRequest, "start 需为 YYYY-MM-DD 日期")
			return
		}
		start = &t
	}
	if v := c.Query("end"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, time.Local)
		if err != nil {
			resp.Fail(c, http.StatusBadRequest, "end 需为 YYYY-MM-DD 日期")
			return
		}
		t = t.AddDate(0, 0, 1) // 含 end 当天：条件用 < end+24h
		end = &t
	}
	pd, err := h.svc.ListUnanswered(c.Request.Context(), page, pageSize, keyword, start, end)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, pd)
}

// rateLimiter 简单固定窗口限流：每 IP 每分钟不超过 limit 次（进程内，重启清零）。
type rateLimiter struct {
	mu    sync.Mutex
	hits  map[string]*ipWindow
	limit int
}

type ipWindow struct {
	count int
	start time.Time
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{hits: make(map[string]*ipWindow), limit: limit}
}

// allow 记一次访问并判断是否放行；map 过大时整体重置防泄漏。
func (l *rateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.hits) > 10000 {
		l.hits = make(map[string]*ipWindow)
	}
	w, ok := l.hits[ip]
	if !ok || now.Sub(w.start) >= time.Minute {
		w = &ipWindow{start: now}
		l.hits[ip] = w
	}
	w.count++
	return w.count <= l.limit
}
