package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"blog/server/internal/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 本文件：问答无命中问题落库（选题回流）。
// 提问在检索层被判定无命中时，把问题原文、归一化键与现成向量落库，同键累加 hits；
// 采集是尽力而为的旁路，任何失败只记日志，绝不影响问答主流程。

// unansweredMinRunes 噪音闸：归一化后不足该字符数的提问视为无效，不入库。
const unansweredMinRunes = 4

// normalizeQuestion 归一化提问：小写、仅保留字母/数字/汉字，作为去重键。
// 「LangChain 怎么部署！」「langchain怎么部署」归为同一键；打错字/换说法
// 靠后续 embedding 余弦聚类合并（本版先按键精确去重）。
func normalizeQuestion(q string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(q) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CaptureUnanswered 落库一条无命中提问（同键 upsert：hits+1 并刷新最近提问）。
// embedding 是检索阶段已算好的向量，存 MySQL 备用列（绝不入 Qdrant 知识库）。
// 用脱离请求的短超时上下文：客户端断开/超时不丢这条记录，卡死也不拖垮问答。
func (s *RagService) CaptureUnanswered(ctx context.Context, question string, embedding []float32) {
	if !s.Enabled() || s.db == nil {
		return
	}
	key := normalizeQuestion(question)
	if utf8.RuneCountInString(key) < unansweredMinRunes {
		return
	}
	// 含 U+FFFD 说明请求体不是合法 UTF-8（如客户端编码错误），归一化键必是乱码，无回流价值
	if strings.ContainsRune(question, utf8.RuneError) {
		return
	}
	cctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var embJSON model.JSON
	if embedding != nil {
		if data, err := json.Marshal(embedding); err == nil {
			embJSON = model.JSON(data)
		}
	}
	now := time.Now()
	row := model.AskUnanswered{
		Question:      truncateRunes(strings.TrimSpace(question), 512),
		NormalizedKey: truncateRunes(key, 191),
		Hits:          1,
		Embedding:     embJSON,
		FirstSeenAt:   now,
		LastSeenAt:    now,
	}
	err := s.db.WithContext(cctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "normalized_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"hits":         gorm.Expr("hits + 1"),
			"question":     row.Question,
			"embedding":    row.Embedding,
			"last_seen_at": now,
		}),
	}).Create(&row).Error
	if err != nil {
		s.log.Warn("无命中问题落库失败", zap.String("key", key), zap.Error(err))
	}
}

// ListUnanswered 管理端无命中问题列表：按提问次数降序，同数次按最近提问时间降序。
// keyword 非空时模糊过滤问题原文与归一化键；start/end 非空时按最近提问时间过滤
// （end 为调用方已 +24h 的开区间上界），选题回流时按主题/时段检索。
func (s *RagService) ListUnanswered(ctx context.Context, page, pageSize int, keyword string, start, end *time.Time) (*PageData, error) {
	page, pageSize = NormalizePage(page, pageSize)
	q := s.db.WithContext(ctx).Model(&model.AskUnanswered{})
	if keyword != "" {
		kw := "%" + escapeLike(keyword) + "%"
		q = q.Where("question LIKE ? OR normalized_key LIKE ?", kw, kw)
	}
	if start != nil {
		q = q.Where("last_seen_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("last_seen_at < ?", *end)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("统计无命中问题数: %w", err)
	}
	items := make([]model.AskUnanswered, 0)
	// embedding 是大字段且列表用不到，显式排除
	err := q.Select("id, question, normalized_key, hits, first_seen_at, last_seen_at").
		Order("hits DESC, last_seen_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("查询无命中问题列表: %w", err)
	}
	return &PageData{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// escapeLike 转义 LIKE 通配符，用户输入里的 % _ \ 按字面匹配。
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
