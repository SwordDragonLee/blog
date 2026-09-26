package model

import (
	"time"
)

// RepoAnalysis LLM 对仓库的分析结果（技术栈、摘要、亮点），文章的归属来源。
type RepoAnalysis struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	TaskID        uint           `gorm:"index" json:"task_id"`
	GitURL        string         `gorm:"size:512" json:"git_url"`
	RepoName      string         `gorm:"size:256" json:"repo_name"`
	DefaultBranch string         `gorm:"size:64" json:"default_branch"`
	HeadCommit    string         `gorm:"size:64" json:"head_commit"`
	TechStack     JSON           `json:"tech_stack" swaggertype:"array,string"`
	Summary       string         `gorm:"type:text" json:"summary"`
	Highlights    JSON           `json:"highlights" swaggertype:"array,string"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (RepoAnalysis) TableName() string { return "repo_analysis" }
