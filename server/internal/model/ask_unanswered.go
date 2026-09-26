package model

import (
	"time"
)

// AskUnanswered 问答无命中问题：选题回流的原料。
// 同一 normalized_key 只保留一行，重复提问累加 hits；
// embedding 是捕获时检索阶段已算好的问题向量（JSON 备用，供后续聚类），
// 纯分析数据，绝不入 Qdrant 知识库、不参与问答检索。
type AskUnanswered struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	Question      string         `gorm:"size:512" json:"question"`             // 最近一次提问原文
	NormalizedKey string         `gorm:"size:191;uniqueIndex" json:"normalized_key"` // 归一化去重键
	Hits          int            `gorm:"not null;default:1" json:"hits"`       // 同键提问次数
	Embedding     JSON           `json:"-" swaggertype:"array,number"`         // 1024 维向量 JSON（聚类备用）
	FirstSeenAt   time.Time      `json:"first_seen_at"`
	LastSeenAt    time.Time      `json:"last_seen_at"`
}

func (AskUnanswered) TableName() string { return "ask_unanswered" }
