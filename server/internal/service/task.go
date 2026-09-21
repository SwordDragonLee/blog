package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"blog/server/internal/model"
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

// TaskPublisher 任务消息投递的抽象：解耦对 mq.Publisher 具体类型的依赖，
// 便于单测中用假实现验证「投递失败 → 任务标记失败」等分支。
type TaskPublisher interface {
	PublishTask(ctx context.Context, taskID uint) error
}

// RagCleaner 删除文章向量的抽象：删除任务时清理 Qdrant 残留向量，可为 nil。
type RagCleaner interface {
	RemoveArticle(ctx context.Context, articleID uint)
}

// TaskService 生成任务的创建、查询与重试。
type TaskService struct {
	db         *gorm.DB
	rdb        *redis.Client
	pub        TaskPublisher
	log        *zap.Logger
	cancelFn   func(taskID uint) // 运行中任务的主动取消回调（main 注入 consumer.CancelTask）
	ragCleaner RagCleaner        // 文章向量清理（main 注入 RagService），可为 nil
}

// NewTaskService 创建任务服务。pub 传 *mq.Publisher 或任意 TaskPublisher 实现。
func NewTaskService(db *gorm.DB, rdb *redis.Client, pub TaskPublisher, log *zap.Logger) *TaskService {
	return &TaskService{db: db, rdb: rdb, pub: pub, log: log}
}

// SetCancelFunc 注入主动取消回调（main 装配时传 consumer.CancelTask）。
func (s *TaskService) SetCancelFunc(f func(taskID uint)) {
	s.cancelFn = f
}

// SetRagCleaner 注入文章向量清理回调（main 装配时传 RagService）。
func (s *TaskService) SetRagCleaner(r RagCleaner) {
	s.ragCleaner = r
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
	// 投递成功才记 queued_at：NULL 即「从未成功入队」，是孤儿扫描的判据
	if err := s.db.WithContext(ctx).Model(t).Update("queued_at", time.Now()).Error; err != nil {
		s.log.Warn("回写 queued_at 出错", zap.Uint("task_id", t.ID), zap.Error(err))
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
	s.applyArticleSummary(ctx, items)
	s.applyLiveProgress(ctx, items)
	return &PageData{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// applyLiveProgress 用 Redis 实时进度覆盖列表中 pending/running 任务的
// step/message/progress。DB 里的进度只在任务开始（1%）与结束（100%）写入，
// 中间值仅存在于 Redis；列表不覆盖的话永远只见 1% 和 100% 两个值。
func (s *TaskService) applyLiveProgress(ctx context.Context, items []model.GenTask) {
	if s.rdb == nil {
		return
	}
	live := make([]int, 0, len(items)) // 需要覆盖的 items 下标
	for i, t := range items {
		if t.Status == model.TaskStatusPending || t.Status == model.TaskStatusRunning {
			live = append(live, i)
		}
	}
	if len(live) == 0 {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, redisReadTimeout)
	defer cancel()
	pipe := s.rdb.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(live))
	for k, idx := range live {
		cmds[k] = pipe.HGetAll(rctx, task.ProgressKey(items[idx].ID))
	}
	if _, err := pipe.Exec(rctx); err != nil {
		return // Redis 不可用时列表退化为 DB 值，不报错
	}
	for k, idx := range live {
		vals, err := cmds[k].Result()
		if err != nil || len(vals) == 0 {
			continue // 无进度记录（如进程重启后），保留 DB 值
		}
		if v, ok := vals["step"]; ok {
			items[idx].Step = v
		}
		if v, ok := vals["message"]; ok {
			items[idx].Message = v
		}
		if p, err := strconv.Atoi(vals["percent"]); err == nil {
			items[idx].Progress = p
		}
	}
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
	if detail.Status == model.TaskStatusSuccess {
		tmp := []model.GenTask{detail.GenTask}
		s.applyArticleSummary(ctx, tmp)
		detail.GenTask = tmp[0] // 切片元素是拷贝，需回填
	}
	return detail, nil
}

// TaskArticles 单个任务的文章状态分布。
type TaskArticles struct {
	Published int
	Draft     int
}

// applyArticleSummary 用文章状态统计覆盖成功任务的完成信息，
// 使发版/下线后任务列表与详情实时反映最新状态（DB 中的完成文案是生成时刻的快照）。
func (s *TaskService) applyArticleSummary(ctx context.Context, tasks []model.GenTask) {
	ids := make([]uint, 0, len(tasks))
	for i := range tasks {
		if tasks[i].Status == model.TaskStatusSuccess {
			ids = append(ids, tasks[i].ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	counts, err := s.articleCounts(ctx, ids)
	if err != nil {
		s.log.Warn("统计任务文章状态失败", zap.Error(err))
		return
	}
	for i := range tasks {
		if tasks[i].Status == model.TaskStatusSuccess {
			tasks[i].Message = articleSummary(counts[tasks[i].ID])
		}
	}
}

// articleCounts 批量统计任务的文章状态分布（draft/published）。
func (s *TaskService) articleCounts(ctx context.Context, taskIDs []uint) (map[uint]TaskArticles, error) {
	out := make(map[uint]TaskArticles, len(taskIDs))
	type row struct {
		TaskID uint
		Status string
		Cnt    int64
	}
	var rows []row
	err := s.db.WithContext(ctx).Model(&model.Article{}).
		Select("repo_analysis.task_id AS task_id, article.status AS status, COUNT(*) AS cnt").
		Joins("JOIN repo_analysis ON repo_analysis.id = article.repo_analysis_id").
		Where("repo_analysis.task_id IN ?", taskIDs).
		Group("repo_analysis.task_id, article.status").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("统计任务文章状态: %w", err)
	}
	for _, r := range rows {
		c := out[r.TaskID]
		switch r.Status {
		case model.ArticleStatusPublished:
			c.Published = int(r.Cnt)
		case model.ArticleStatusDraft:
			c.Draft = int(r.Cnt)
		}
		out[r.TaskID] = c
	}
	return out, nil
}

// articleSummary 渲染任务完成信息。
func articleSummary(c TaskArticles) string {
	if c.Published+c.Draft == 0 {
		return "生成完成，未产出文章"
	}
	return fmt.Sprintf("生成完成：%d 篇已发布、%d 篇待审核", c.Published, c.Draft)
}

// Retry 重新投递失败任务：仅 failed 状态可重试；重置进度与尝试计数、
// 保留历史日志（追加重试分隔线，便于回溯上次失败原因）后重新入队。
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
		// 重置为「未入队」：补投成功后再写入 queued_at，
		// 若此间进程崩溃，重启时的孤儿扫描仍能发现并补投该任务
		"queued_at": nil,
	}
	if err := s.db.WithContext(ctx).Model(&t).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("重置任务 %d: %w", id, err)
	}
	// 保留历史日志供回溯上次失败原因：只清进度与尝试计数，并追加重试分隔线
	if s.rdb != nil {
		rctx, cancel := context.WithTimeout(ctx, redisReadTimeout)
		_ = s.rdb.Del(rctx, task.ProgressKey(id), task.AttemptsKey(id)).Err()
		cancel()
		task.AppendLog(s.rdb, id, "══════ 手动重试：任务重新入队 ══════")
	}

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
	if err := s.db.WithContext(ctx).Model(&t).Update("queued_at", time.Now()).Error; err != nil {
		s.log.Warn("回写 queued_at 出错", zap.Uint("task_id", id), zap.Error(err))
	}
	s.log.Info("失败任务已重新投递", zap.Uint("task_id", id))
	t.Status = model.TaskStatusPending
	t.Progress = 0
	t.Step = "排队中"
	t.Message = "任务已重新入队，等待执行"
	t.Error = ""
	return &t, nil
}

// Cancel 取消任务：仅 pending/running 可取消。
// pending：落库 canceled，消息被消费时流水线检测状态后跳过；
// running：落库 canceled 并中断执行中的流水线（取消其派生 context）。
func (s *TaskService) Cancel(ctx context.Context, id uint) (*model.GenTask, error) {
	var t model.GenTask
	err := s.db.WithContext(ctx).First(&t, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 任务 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询任务 %d: %w", id, err)
	}
	if t.Status != model.TaskStatusPending && t.Status != model.TaskStatusRunning {
		return nil, fmt.Errorf("%w: 仅排队中或运行中的任务可取消，当前状态为 %s", ErrConflict, t.Status)
	}
	if err := s.db.WithContext(ctx).Model(&t).Updates(map[string]any{
		"status":  model.TaskStatusCanceled,
		"step":    "已取消",
		"message": "任务已取消",
		"error":   "",
	}).Error; err != nil {
		return nil, fmt.Errorf("更新任务 %d: %w", id, err)
	}
	s.clearProgress(ctx, id)
	if s.cancelFn != nil {
		s.cancelFn(id) // 任务不在执行时为空操作
	}
	s.log.Info("任务已取消", zap.Uint("task_id", id))
	return s.reloadTask(ctx, id)
}

// Delete 删除任务及其衍生数据（repo_analysis、草稿文章、配图、Redis 进度）。
// 存在已发布文章时拒绝删除（需先下线），避免前台内容被连带清除；
// 排队中/运行中的任务先自动取消再删。
func (s *TaskService) Delete(ctx context.Context, id uint) error {
	var t model.GenTask
	err := s.db.WithContext(ctx).First(&t, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: 任务 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("查询任务 %d: %w", id, err)
	}
	if t.Status == model.TaskStatusPending || t.Status == model.TaskStatusRunning {
		if _, err := s.Cancel(ctx, id); err != nil {
			return fmt.Errorf("取消任务 %d: %w", id, err)
		}
	}

	// 已发布文章保护：删任务不能动前台内容
	var analysisIDs []uint
	if err := s.db.WithContext(ctx).Model(&model.RepoAnalysis{}).
		Where("task_id = ?", id).Pluck("id", &analysisIDs).Error; err != nil {
		return fmt.Errorf("查询任务 %d 分析记录: %w", id, err)
	}
	if len(analysisIDs) > 0 {
		var published int64
		if err := s.db.WithContext(ctx).Model(&model.Article{}).
			Where("repo_analysis_id IN ? AND status = ?", analysisIDs, model.ArticleStatusPublished).
			Count(&published).Error; err != nil {
			return fmt.Errorf("统计已发布文章: %w", err)
		}
		if published > 0 {
			return fmt.Errorf("%w: 任务 %d 存在已发布文章，请先下线后再删除任务", ErrConflict, id)
		}
	}

	// 事务外先收集任务下的文章 ID：事务提交后用于清理 Qdrant 残留向量。
	// 正常情况下 draft 文章没有向量，这里兜底「发版后立刻退回 draft」竞态窗口内
	// 异步向量化 upsert 晚于 RemoveArticle 落地留下的孤儿向量。
	artIDs := make([]uint, 0)
	if len(analysisIDs) > 0 {
		if err := s.db.WithContext(ctx).Model(&model.Article{}).
			Where("repo_analysis_id IN ?", analysisIDs).Pluck("id", &artIDs).Error; err != nil {
			return fmt.Errorf("查询任务 %d 文章: %w", id, err)
		}
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(analysisIDs) > 0 {
			if len(artIDs) > 0 {
				if err := tx.Where("article_id IN ?", artIDs).
					Delete(&model.SvgAsset{}).Error; err != nil {
					return err
				}
				if err := tx.Where("id IN ?", artIDs).
					Delete(&model.Article{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("task_id = ?", id).
				Delete(&model.RepoAnalysis{}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&model.GenTask{}, id).Error
	}); err != nil {
		return fmt.Errorf("删除任务 %d: %w", id, err)
	}

	// Qdrant 删除按文章 ID 过滤、无命中为空操作，幂等安全
	if s.ragCleaner != nil {
		for _, artID := range artIDs {
			s.ragCleaner.RemoveArticle(ctx, artID)
		}
	}

	s.clearProgress(ctx, id)
	if s.cancelFn != nil {
		s.cancelFn(id) // 兜底：中断可能仍在收尾的流水线
	}
	s.log.Info("任务已删除", zap.Uint("task_id", id),
		zap.Int("repo_analyses", len(analysisIDs)))
	return nil
}

// reloadTask 重新读取任务行（Cancel 落库后返回最新状态）。
func (s *TaskService) reloadTask(ctx context.Context, id uint) (*model.GenTask, error) {
	var t model.GenTask
	if err := s.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, fmt.Errorf("查询任务 %d: %w", id, err)
	}
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

// RecoverUnqueued 孤儿任务补偿：Create/Retry 是「先落库、后投递」两步，
// 进程在两步之间崩溃会留下 status=pending 且 queued_at 为 NULL 的任务，
// 其消息从未进入 MQ，重启后永远不会被执行。启动时扫描这类任务并补投。
//
// 正常排队中的任务 queued_at 非空，不会被误扫；补投仍失败的（如 MQ 不可用）
// 保持 NULL，下次重启再次尝试。返回补投成功的任务数。
func (s *TaskService) RecoverUnqueued(ctx context.Context) (int, error) {
	var ids []uint
	err := s.db.WithContext(ctx).Model(&model.GenTask{}).
		Where("status = ? AND queued_at IS NULL", model.TaskStatusPending).
		Pluck("id", &ids).Error
	if err != nil {
		return 0, fmt.Errorf("扫描孤儿任务: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	recovered := 0
	for _, id := range ids {
		if err := s.pub.PublishTask(ctx, id); err != nil {
			// MQ 不可用等场景：保留 NULL 标记，下次启动重试
			s.log.Warn("孤儿任务补投失败", zap.Uint("task_id", id), zap.Error(err))
			continue
		}
		if err := s.db.WithContext(ctx).Model(&model.GenTask{}).
			Where("id = ?", id).Update("queued_at", time.Now()).Error; err != nil {
			s.log.Warn("回写 queued_at 出错", zap.Uint("task_id", id), zap.Error(err))
			continue
		}
		recovered++
	}
	if recovered > 0 {
		s.log.Info("已补投孤儿任务消息", zap.Int("count", recovered), zap.Int("total", len(ids)))
	}
	return recovered, nil
}
