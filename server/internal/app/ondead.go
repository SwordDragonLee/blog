package app

import (
	"context"
	"fmt"
	"time"

	"blog/server/internal/model"
	"blog/server/internal/task"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// newOnDead 消息重试超限进入死信队列前的回调：把任务标记为 failed，
// 并同步 Redis 进度与任务日志供管理端轮询展示。
func newOnDead(db *gorm.DB, rdb *redis.Client, log *zap.Logger) func(ctx context.Context, taskID uint) {
	return func(ctx context.Context, taskID uint) {
		// 保留流水线最后一次写入的真实错误（如「克隆仓库超时」），避免被固定文案覆盖
		var t model.GenTask
		errMsg := "任务多次重试仍失败，已进入死信队列"
		if err := db.WithContext(ctx).First(&t, taskID).Error; err == nil && t.Error != "" {
			errMsg = fmt.Sprintf("重试超限，最后一次错误：%s", t.Error)
		}
		if err := db.WithContext(ctx).Model(&model.GenTask{}).
			Where("id = ?", taskID).
			Updates(map[string]any{
				"status":  model.TaskStatusFailed,
				"step":    "失败",
				"message": "任务失败",
				"error":   errMsg,
			}).Error; err != nil {
			log.Error("标记任务失败状态出错", zap.Uint("task_id", taskID), zap.Error(err))
		}
		if rdb != nil {
			pctx, cancel := context.WithTimeout(context.Background(), progressWriteTimeout)
			defer cancel()
			if err := rdb.HSet(pctx, task.ProgressKey(taskID), map[string]any{
				"step":       "失败",
				"percent":    0,
				"message":    errMsg,
				"updated_at": time.Now().Format(time.RFC3339),
			}).Err(); err != nil {
				log.Warn("写入任务进度失败", zap.Uint("task_id", taskID), zap.Error(err))
			}
			task.AppendLog(rdb, taskID, "任务失败：%s", errMsg)
		}
		log.Error("任务进入死信队列", zap.Uint("task_id", taskID), zap.String("error", errMsg))
	}
}
