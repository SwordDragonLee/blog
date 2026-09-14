package task

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	progressTTL   = 7 * 24 * time.Hour // 进度/日志 key 的兜底过期时间
	maxLogEntries = 200                // 任务日志保留条数
	writeTimeout  = 3 * time.Second    // 单次 Redis 写超时
)

// ProgressKey 任务实时进度的 Redis key（hash：step/percent/message/updated_at）。
func ProgressKey(taskID uint) string { return fmt.Sprintf("task:%d:progress", taskID) }

// LogsKey 任务执行日志的 Redis key（list，追加式，保留最近 maxLogEntries 条）。
func LogsKey(taskID uint) string { return fmt.Sprintf("task:%d:logs", taskID) }

// progressReporter 把任务实时进度写 Redis，供管理平台轮询。
// Redis 不可用时仅记录告警，不阻塞流水线。
type progressReporter struct {
	rdb    *redis.Client
	taskID uint
	log    *zap.Logger
}

// step 更新当前步骤、进度百分比与说明，并追加一条日志。
func (p *progressReporter) step(percent int, step, message string) {
	if p == nil || p.rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	key := ProgressKey(p.taskID)
	if err := p.rdb.HSet(ctx, key, map[string]any{
		"step":       step,
		"percent":    percent,
		"message":    message,
		"updated_at": time.Now().Format(time.RFC3339),
	}).Err(); err != nil {
		p.log.Warn("写入任务进度失败", zap.Uint("task_id", p.taskID), zap.Error(err))
	}
	_ = p.rdb.Expire(ctx, key, progressTTL).Err()
	p.logf("[%s] %d%% %s", step, percent, message)
}

// logf 追加一条执行日志（带时间戳），超长自动截断。
func (p *progressReporter) logf(format string, args ...any) {
	if p == nil || p.rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	line := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	key := LogsKey(p.taskID)
	if err := p.rdb.RPush(ctx, key, line).Err(); err != nil {
		p.log.Warn("写入任务日志失败", zap.Uint("task_id", p.taskID), zap.Error(err))
		return
	}
	_ = p.rdb.LTrim(ctx, key, -maxLogEntries, -1).Err()
	_ = p.rdb.Expire(ctx, key, progressTTL).Err()
}
