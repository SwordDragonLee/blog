package model

import "time"

// 任务状态：pending 排队 → running 执行 → success / failed / canceled 终态。
const (
	TaskStatusPending  = "pending"
	TaskStatusRunning  = "running"
	TaskStatusSuccess  = "success"
	TaskStatusFailed   = "failed"
	TaskStatusCanceled = "canceled"
)

// GenTask 一次仓库博客生成任务的执行记录。
type GenTask struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	GitURL         string `gorm:"size:512" json:"git_url"`
	Status         string `gorm:"size:16;index;default:pending" json:"status"`
	Progress       int    `json:"progress"`
	Step           string `gorm:"size:64" json:"step"`
	Message        string `gorm:"size:512" json:"message"`
	Error          string `gorm:"size:1024" json:"error"`
	RepoAnalysisID *uint  `json:"repo_analysis_id"`
	// QueuedAt 消息成功投递到 MQ 的时刻；NULL 表示尚未投递成功（孤儿任务，
	// 由启动扫描补投），是「落库」与「投递」两步之间崩溃的补偿依据。
	QueuedAt  *time.Time `gorm:"index" json:"queued_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (GenTask) TableName() string { return "gen_task" }
