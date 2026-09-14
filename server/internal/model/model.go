// Package model 定义 GORM 数据模型与业务常量。
package model

import (
	"time"

	"gorm.io/datatypes"
)

type AdminUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:64;uniqueIndex" json:"username"`
	PasswordHash string    `gorm:"size:128" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

func (AdminUser) TableName() string { return "admin_user" }

// 任务状态
const (
	TaskStatusPending = "pending"
	TaskStatusRunning = "running"
	TaskStatusSuccess = "success"
	TaskStatusFailed  = "failed"
)

type GenTask struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	GitURL         string    `gorm:"size:512" json:"git_url"`
	Status         string    `gorm:"size:16;index;default:pending" json:"status"`
	Progress       int       `json:"progress"`
	Step           string    `gorm:"size:64" json:"step"`
	Message        string    `gorm:"size:512" json:"message"`
	Error          string    `gorm:"size:1024" json:"error"`
	RepoAnalysisID *uint     `json:"repo_analysis_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (GenTask) TableName() string { return "gen_task" }

type RepoAnalysis struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	TaskID        uint           `gorm:"index" json:"task_id"`
	GitURL        string         `gorm:"size:512" json:"git_url"`
	RepoName      string         `gorm:"size:256" json:"repo_name"`
	DefaultBranch string         `gorm:"size:64" json:"default_branch"`
	HeadCommit    string         `gorm:"size:64" json:"head_commit"`
	TechStack     datatypes.JSON `json:"tech_stack"`
	Summary       string         `gorm:"type:text" json:"summary"`
	Highlights    datatypes.JSON `json:"highlights"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (RepoAnalysis) TableName() string { return "repo_analysis" }

// 文章状态：draft（待审核）-> published -> offline 回到 draft
const (
	ArticleStatusDraft     = "draft"
	ArticleStatusPublished = "published"
)

type Article struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	RepoAnalysisID uint           `gorm:"index" json:"repo_analysis_id"`
	Title          string         `gorm:"size:256" json:"title"`
	Slug           string         `gorm:"size:128;uniqueIndex" json:"slug"`
	Summary        string         `gorm:"size:512" json:"summary"`
	ContentMD      string         `gorm:"mediumtext" json:"content_md"`
	WordCount      int            `json:"word_count"`
	Tags           datatypes.JSON `json:"tags"`
	Status         string         `gorm:"size:16;index;default:draft" json:"status"`
	SortOrder      int            `json:"sort_order"`
	CoverAssetID   *uint          `json:"cover_asset_id"`
	PublishedAt    *time.Time     `json:"published_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (Article) TableName() string { return "article" }

// 配图类型
const (
	FigureKindCover        = "cover"
	FigureKindArchitecture = "architecture"
	FigureKindFlow         = "flow"
	FigureKindCompare      = "compare"
	FigureKindTimeline     = "timeline"
)

type SvgAsset struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	ArticleID   uint           `gorm:"index" json:"article_id"`
	Placeholder string         `gorm:"size:32;index" json:"placeholder"` // 正文中的 {{figure:xxx}} 标识
	Kind        string         `gorm:"size:32" json:"kind"`
	Title       string         `gorm:"size:256" json:"title"`
	Spec        datatypes.JSON `json:"spec"`
	SVGContent  string         `gorm:"mediumtext" json:"-"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (SvgAsset) TableName() string { return "svg_asset" }
