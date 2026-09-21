package model

import (
	"time"

	"gorm.io/datatypes"
)

// 文章状态：draft（待审核）→ publish → published → offline 回到 draft。
const (
	ArticleStatusDraft     = "draft"
	ArticleStatusPublished = "published"
)

// Article 生成的技术文章，经审核发版后在前台展示。
type Article struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	RepoAnalysisID uint           `gorm:"index" json:"repo_analysis_id"`
	Title          string         `gorm:"size:256" json:"title"`
	Slug           string         `gorm:"size:128;uniqueIndex" json:"slug"`
	Summary        string         `gorm:"size:512" json:"summary"`
	ContentMD      string         `gorm:"mediumtext" json:"content_md"`
	WordCount      int            `json:"word_count"`
	LikeCount      int            `gorm:"not null;default:0" json:"like_count"`
	Tags           datatypes.JSON `json:"tags"`
	Status         string         `gorm:"size:16;index;default:draft" json:"status"`
	SortOrder      int            `json:"sort_order"`
	CoverAssetID   *uint          `json:"cover_asset_id"`
	PublishedAt    *time.Time     `json:"published_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (Article) TableName() string { return "article" }
