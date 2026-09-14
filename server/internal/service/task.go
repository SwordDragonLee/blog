package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"blog/server/internal/model"
	"blog/server/internal/mq"
	"blog/server/internal/task"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	redisReadTimeout = 2 * time.Second // 读取任务进度/日志的超时
)

// PageData 统一分页响应结构 {items,total,page,page_size}。
type PageData struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// NormalizePage 归一化分页参数：page>=1，pageSize 默认 10、上限 100。
func NormalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

// TaskService 生成任务的创建、查询与重试。
type TaskService struct {
	db  *gorm.DB
	rdb *redis.Client
	pub *mq.Publisher
	log *zap.Logger
}

// NewTaskService 创建任务服务。
func NewTaskService(db *gorm.DB, rdb *redis.Client, pub *mq.Publisher, log *zap.Logger) *TaskService {
	return &TaskService{db: db, rdb: rdb, pub: pub, log: log}
}

// TaskDetail 任务详情：DB 记录 + Redis 实时进度 + 执行日志。
type TaskDetail struct {
	model.GenTask
	Logs []string `json:"logs"`
}

// Create 校验 git_url，落库 pending 并投递 MQ 消息。
func (s *TaskService) Create(ctx context.Context, gitURL string) (*model.GenTask, error) {
	gitURL = strings.TrimSpace(gitURL)
	if !ValidGitURL(gitURL) {
		return nil, fmt.Errorf("%w: git_url 需以 https://、http:// 或 git@ 开头", ErrInvalid)
	}
	t := &model.GenTask{
		GitURL:  gitURL,
		Status:  model.TaskStatusPending,
		Step:    "排队中",
		Message: "任务已创建，等待执行",
	}
	if err := s.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil, fmt.Errorf("创建任务: %w", err)
	}
	if err := s.pub.PublishTask(ctx, t.ID); err != nil {
		// 投递失败：任务标记失败，之后可通过 retry 重新投递
		errMsg := truncateRunes("投递任务消息失败: "+err.Error(), 1000)
		if uerr := s.db.WithContext(ctx).Model(t).Updates(map[string]any{
			"status": model.TaskStatusFailed,
			"error":  errMsg,
		}).Error; uerr != nil {
			s.log.Warn("回写任务失败状态出错", zap.Uint("task_id", t.ID), zap.Error(uerr))
		}
		return nil, fmt.Errorf("投递任务消息: %w", err)
	}
	s.log.Info("任务已创建并投递", zap.Uint("task_id", t.ID), zap.String("git_url", gitURL))
	return t, nil
}

// List 任务列表（按 id 倒序分页）。
func (s *TaskService) List(ctx context.Context, page, pageSize int) (*PageData, error) {
	page, pageSize = NormalizePage(page, pageSize)
	q := s.db.WithContext(ctx).Model(&model.GenTask{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("统计任务数: %w", err)
	}
	items := make([]model.GenTask, 0)
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("查询任务列表: %w", err)
	}
	return &PageData{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// Get 任务详情：DB 记录为主，pending/running 时用 Redis 进度 Hash 覆盖
// step/percent/message，并合并执行日志列表（管理平台 2s 轮询）。
func (s *TaskService) Get(ctx context.Context, id uint) (*TaskDetail, error) {
	var t model.GenTask
	err := s.db.WithContext(ctx).First(&t, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 任务 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询任务 %d: %w", id, err)
	}
	detail := &TaskDetail{GenTask: t, Logs: []string{}}

	if s.rdb == nil {
		return detail, nil
	}
	rctx, cancel := context.WithTimeout(ctx, redisReadTimeout)
	defer cancel()

	if vals, err := s.rdb.HGetAll(rctx, task.ProgressKey(id)).Result(); err == nil && len(vals) > 0 {
		// 进行中的任务以 Redis 实时进度为准；终态以 DB 为准
		if t.Status == model.TaskStatusPending || t.Status == model.TaskStatusRunning {
			if v, ok := vals["step"]; ok {
				detail.Step = v
			}
			if v, ok := vals["message"]; ok {
				detail.Message = v
			}
			if p, err := strconv.Atoi(vals["percent"]); err == nil {
				detail.Progress = p
			}
		}
	}
	if logs, err := s.rdb.LRange(rctx, task.LogsKey(id), 0, -1).Result(); err == nil && len(logs) > 0 {
		detail.Logs = logs
	}
	return detail, nil
}

// Retry 重新投递失败任务：仅 failed 状态可重试；重置进度并清空 Redis
// 进度/日志后重新入队。
func (s *TaskService) Retry(ctx context.Context, id uint) (*model.GenTask, error) {
	var t model.GenTask
	err := s.db.WithContext(ctx).First(&t, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 任务 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询任务 %d: %w", id, err)
	}
	if t.Status != model.TaskStatusFailed {
		return nil, fmt.Errorf("%w: 仅失败任务可重试，当前状态为 %s", ErrConflict, t.Status)
	}
	updates := map[string]any{
		"status":   model.TaskStatusPending,
		"progress": 0,
		"step":     "排队中",
		"message":  "任务已重新入队，等待执行",
		"error":    "",
	}
	if err := s.db.WithContext(ctx).Model(&t).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("重置任务 %d: %w", id, err)
	}
	s.clearProgress(ctx, id)

	if err := s.pub.PublishTask(ctx, id); err != nil {
		errMsg := truncateRunes("投递任务消息失败: "+err.Error(), 1000)
		if uerr := s.db.WithContext(ctx).Model(&t).Updates(map[string]any{
			"status": model.TaskStatusFailed,
			"error":  errMsg,
		}).Error; uerr != nil {
			s.log.Warn("回写任务失败状态出错", zap.Uint("task_id", id), zap.Error(uerr))
		}
		return nil, fmt.Errorf("投递任务消息: %w", err)
	}
	s.log.Info("失败任务已重新投递", zap.Uint("task_id", id))
	t.Status = model.TaskStatusPending
	t.Progress = 0
	t.Step = "排队中"
	t.Message = "任务已重新入队，等待执行"
	t.Error = ""
	return &t, nil
}

// clearProgress 清空任务的 Redis 进度与日志（重跑前重置，失败不阻断）。
func (s *TaskService) clearProgress(ctx context.Context, id uint) {
	if s.rdb == nil {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, redisReadTimeout)
	defer cancel()
	if err := s.rdb.Del(rctx, task.ProgressKey(id), task.LogsKey(id)).Err(); err != nil {
		s.log.Warn("清理任务 Redis 进度失败", zap.Uint("task_id", id), zap.Error(err))
	}
}

// ValidGitURL 校验 git 仓库地址：https://、http:// 或 git@ 开头。
func ValidGitURL(u string) bool {
	if u == "" || len(u) > 512 {
		return false
	}
	lower := strings.ToLower(u)
	return strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "git@")
}
