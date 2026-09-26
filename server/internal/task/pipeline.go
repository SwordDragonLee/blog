// Package task 实现生成任务流水线：克隆仓库 → 静态分析采样 → LLM 分析 →
// 逐篇写作 → 渲染 SVG 配图 → 文章落库（draft），实时进度写 Redis。
// 由 MQ consumer 调用，pipeline 按 slug upsert / 按 article_id 重建配图，可安全重跑。
package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"blog/server/internal/analyzer"
	"blog/server/internal/config"
	"blog/server/internal/llm"
	"blog/server/internal/model"
	"blog/server/internal/mq"
	"blog/server/internal/svggen"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Pipeline 聚合流水线依赖。
type Pipeline struct {
	cfg config.Config
	db  *gorm.DB
	rdb *redis.Client
	llm *llm.Client
	log *zap.Logger
}

// New 创建流水线。
func New(cfg config.Config, db *gorm.DB, rdb *redis.Client, client *llm.Client, log *zap.Logger) *Pipeline {
	return &Pipeline{cfg: cfg, db: db, rdb: rdb, llm: client, log: log}
}

// Handle 执行一次完整生成流程。失败返回错误（由 MQ 决定重投/死信）；
// 因服务关闭被取消时不标记失败，消息未 ack 会被 RabbitMQ 重新投递。
func (p *Pipeline) Handle(ctx context.Context, taskID uint) error {
	var t model.GenTask
	if err := p.db.WithContext(ctx).First(&t, taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 任务已删除：ack 丢弃消息，不重试
			return fmt.Errorf("%w: 任务 %d 已删除", mq.ErrTaskCanceled, taskID)
		}
		return fmt.Errorf("查询任务 %d: %w", taskID, err)
	}
	if t.Status == model.TaskStatusCanceled {
		// 任务在排队期间被取消：跳过执行
		return fmt.Errorf("%w: 任务 %d", mq.ErrTaskCanceled, taskID)
	}
	if strings.TrimSpace(t.GitURL) == "" {
		return fmt.Errorf("任务 %d 缺少 git_url", taskID)
	}

	prog := &progressReporter{rdb: p.rdb, taskID: taskID, log: p.log}
	// 尝试计数：MQ 重投会再次进入 Handle，计数让管理端能分辨各次尝试
	attempts := int64(1)
	if p.rdb != nil {
		actx, acancel := context.WithTimeout(context.Background(), writeTimeout)
		if n, err := p.rdb.Incr(actx, AttemptsKey(taskID)).Result(); err == nil {
			attempts = n
		}
		_ = p.rdb.Expire(actx, AttemptsKey(taskID), progressTTL).Err()
		acancel()
	}
	if attempts > 1 {
		prog.logf("══════ 第 %d 次尝试 ══════", attempts)
	}
	prog.step(1, "开始", fmt.Sprintf("任务开始执行（第 %d 次尝试）", attempts))
	p.updateTask(ctx, taskID, map[string]any{
		"status": model.TaskStatusRunning, "progress": 1, "step": "开始",
		"message": "任务开始执行", "error": "",
	})

	// ① 克隆仓库（5%）
	prog.step(5, "克隆仓库", "开始克隆 "+t.GitURL)
	info, err := analyzer.Clone(ctx, t.GitURL, p.cfg.Task.WorkDir,
		time.Duration(p.cfg.Task.CloneTimeoutSeconds)*time.Second, p.cfg.Task.Proxy)
	if err != nil {
		return p.fail(ctx, prog, taskID, fmt.Errorf("克隆仓库: %w", err))
	}
	defer analyzer.Cleanup(info.Dir)
	prog.logf("克隆完成：%s @ %s", info.RepoName, info.HeadCommit)

	// ② 仓库分析（15%）：语言统计 / 目录树 / README / 代码采样
	prog.step(15, "仓库分析", "扫描代码库并采样核心文件")
	det, err := analyzer.Detect(info.Dir)
	if err != nil {
		return p.fail(ctx, prog, taskID, fmt.Errorf("扫描仓库: %w", err))
	}
	files := analyzer.Sample(info.Dir, det, p.cfg.Task.MaxFiles, p.cfg.Task.MaxFileKB, p.cfg.Task.TotalBudgetKB)
	digest := buildDigest(info, det, files)
	prog.logf("识别技术栈 %d 项，采样 %d 个文件，材料 %d 字节", len(det.TechStack), len(files), len(digest))

	// ③ LLM 分析（30%）
	prog.step(30, "LLM 分析", "调用大模型分析仓库与文章选题")
	analysis, err := p.analyzeRepo(ctx, digest)
	if err != nil {
		return p.fail(ctx, prog, taskID, fmt.Errorf("LLM 分析仓库: %w", err))
	}
	repoName := firstNonEmpty(analysis.RepoName, info.RepoName)
	prog.logf("分析完成：技术栈 %s", strings.Join(analysis.TechStack, "、"))

	ra := &model.RepoAnalysis{
		TaskID:        taskID,
		GitURL:        t.GitURL,
		RepoName:      repoName,
		DefaultBranch: info.DefaultBranch,
		HeadCommit:    info.HeadCommit,
		TechStack:     jsonify(firstNonEmptySlice(analysis.TechStack, det.TechStack)),
		Summary:       analysis.Summary,
		Highlights:    jsonify(analysis.Highlights),
	}
	if err := p.db.WithContext(ctx).Create(ra).Error; err != nil {
		return p.fail(ctx, prog, taskID, fmt.Errorf("保存仓库分析: %w", err))
	}
	p.updateTask(ctx, taskID, map[string]any{"repo_analysis_id": ra.ID})

	// 幂等清理：删除本任务历史尝试遗留的草稿文章及配图（已发布的不动），
	// 避免重试后新旧文章混杂。
	p.cleanupPreviousDrafts(ctx, taskID, prog)

	// ④ 逐篇写作 + ⑤ 渲染配图（30% → 90%）
	analysisJSON := jsonify(analysis)
	plans := analysis.ArticlePlan
	if len(plans) > p.cfg.Task.ArticleCount {
		plans = plans[:p.cfg.Task.ArticleCount]
	}
	total := len(plans)
	done := 0
	for i, plan := range plans {
		percent := 30 + (i+1)*60/total
		prog.step(percent, "撰写文章", fmt.Sprintf("(%d/%d) %s", i+1, total, plan.Title))
		artOut, err := p.writeArticle(ctx, repoName, analysisJSON, plan)
		if err != nil {
			return p.fail(ctx, prog, taskID, fmt.Errorf("撰写文章 %d/%d: %w", i+1, total, err))
		}
		art, err := p.saveArticle(ctx, ra.ID, artOut, i)
		if err != nil {
			return p.fail(ctx, prog, taskID, fmt.Errorf("保存文章「%s」: %w", artOut.Title, err))
		}
		if err := p.saveFiguresAndCover(ctx, art, artOut.Figures); err != nil {
			return p.fail(ctx, prog, taskID, fmt.Errorf("保存配图「%s」: %w", art.Title, err))
		}
		prog.logf("文章已入库：《%s》（%d 字，配图 %d 张）", art.Title, art.WordCount, len(artOut.Figures))
		done++
	}

	// ⑥ 完成（100%）：文章均已落库为 draft。审核属于文章状态机（draft→published），
	// 是任务结束之后的人工环节，不作为任务的一个阶段。
	prog.step(100, "完成", fmt.Sprintf("生成完成，共 %d 篇文章待审核", done))
	p.updateTask(ctx, taskID, map[string]any{
		"status": model.TaskStatusSuccess, "progress": 100, "step": "完成",
		"message": fmt.Sprintf("生成完成，共 %d 篇文章待审核", done), "error": "",
	})
	return nil
}

// analyzeRepo 调用 LLM 第一轮分析，校验文章选题非空。
func (p *Pipeline) analyzeRepo(ctx context.Context, digest string) (*llm.AnalysisOut, error) {
	system, user := llm.BuildAnalysisPrompt(digest, p.cfg.Task.ArticleCount)
	var out llm.AnalysisOut
	err := p.llm.ChatJSON(ctx, system, user, &out, func() error {
		if len(out.ArticlePlan) == 0 {
			return errors.New("article_plan 不能为空")
		}
		return nil
	})
	return &out, err
}

// minArticleWords 单篇最低字数门槛，低于此值触发带反馈的重写（重写由 ChatJSON 的校验重试机制承担）。
const minArticleWords = 1000

// writeArticle 调用 LLM 第二轮写作，校验标题/正文/slug 与配图结构。
func (p *Pipeline) writeArticle(ctx context.Context, repoName string, analysisJSON []byte, plan llm.ArticlePlanOut) (*llm.ArticleOut, error) {
	system, user := llm.BuildArticlePrompt(repoName, string(analysisJSON), string(jsonify(plan)))
	var out llm.ArticleOut
	err := p.llm.ChatJSON(ctx, system, user, &out, func() error {
		if strings.TrimSpace(out.Title) == "" {
			return errors.New("title 不能为空")
		}
		if strings.TrimSpace(out.Slug) == "" {
			return errors.New("slug 不能为空")
		}
		if strings.TrimSpace(out.Markdown) == "" {
			return errors.New("markdown 不能为空")
		}
		if wc := countWords(out.Markdown); wc < minArticleWords {
			return fmt.Errorf("字数不足：仅 %d 字（要求不少于 %d 字），请充分展开正文，补充代码示例与原理讲解", wc, minArticleWords)
		}
		if len(out.Figures) == 0 {
			return errors.New("至少需要 1 张配图，请按提纲安排 2-4 张配图并保证占位符一致")
		}
		seen := map[string]bool{}
		for i, f := range out.Figures {
			if strings.TrimSpace(f.ID) == "" {
				return fmt.Errorf("figures[%d].id 不能为空", i)
			}
			if seen[f.ID] {
				return fmt.Errorf("figures[%d].id 重复: %s", i, f.ID)
			}
			seen[f.ID] = true
			switch f.Kind {
			case model.FigureKindArchitecture, model.FigureKindFlow,
				model.FigureKindCompare, model.FigureKindTimeline:
			default:
				return fmt.Errorf("figures[%d].kind 非法: %s", i, f.Kind)
			}
		}
		return nil
	})
	return &out, err
}

// saveArticle 按 slug upsert 文章（重跑安全）：存在则覆盖内容并回到 draft，
// 同步删除旧配图；不存在则新建。
func (p *Pipeline) saveArticle(ctx context.Context, analysisID uint, out *llm.ArticleOut, sortOrder int) (*model.Article, error) {
	tags := jsonify(out.Tags)
	for attempt := 0; attempt < 3; attempt++ {
		slug := p.uniqueSlug(ctx, out.Slug)
		var art model.Article
		err := p.db.WithContext(ctx).Where("slug = ?", slug).First(&art).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !isNew {
			return nil, err
		}
		art.RepoAnalysisID = analysisID
		art.Title = out.Title
		art.Slug = slug
		art.Summary = out.Summary
		art.ContentMD = out.Markdown
		art.WordCount = countWords(out.Markdown)
		art.Tags = tags
		art.Status = model.ArticleStatusDraft // 重新生成后回到待审核
		art.SortOrder = sortOrder
		if isNew {
			err = p.db.WithContext(ctx).Create(&art).Error
		} else {
			err = p.db.WithContext(ctx).Save(&art).Error
		}
		if err != nil {
			if isDuplicateErr(err) { // 并发撞 slug：换后缀重试
				continue
			}
			return nil, err
		}
		// SVG 按 article_id 重建（幂等）
		if err := p.db.WithContext(ctx).Where("article_id = ?", art.ID).
			Delete(&model.SvgAsset{}).Error; err != nil {
			return nil, err
		}
		return &art, nil
	}
	return nil, fmt.Errorf("slug 唯一性冲突，重试 3 次仍未成功")
}

// saveFiguresAndCover 渲染并保存文章配图与封面。
func (p *Pipeline) saveFiguresAndCover(ctx context.Context, art *model.Article, figures []llm.FigureOut) error {
	for _, f := range figures {
		svg, err := svggen.Render(f.Kind, f.Title, string(f.Data))
		if err != nil {
			// 渲染是确定性流程，失败仅跳过该图，不阻塞整篇文章
			p.log.Warn("渲染配图失败，跳过",
				zap.Uint("article_id", art.ID), zap.String("figure", f.ID), zap.Error(err))
			continue
		}
		asset := &model.SvgAsset{
			ArticleID:   art.ID,
			Placeholder: f.ID,
			Kind:        f.Kind,
			Title:       f.Title,
			Spec:        jsonify(f),
			SVGContent:  svg,
		}
		if err := p.db.WithContext(ctx).Create(asset).Error; err != nil {
			return err
		}
	}

	// 封面卡片
	var tags []string
	_ = json.Unmarshal(art.Tags, &tags)
	in := svggen.CoverInput{Title: art.Title, Tags: tags, Subtitle: art.Summary}
	asset := &model.SvgAsset{
		ArticleID:   art.ID,
		Placeholder: "cover",
		Kind:        model.FigureKindCover,
		Title:       art.Title,
		Spec:        jsonify(in),
		SVGContent:  svggen.RenderCover(in),
	}
	if err := p.db.WithContext(ctx).Create(asset).Error; err != nil {
		return err
	}
	return p.db.WithContext(ctx).Model(art).Update("cover_asset_id", asset.ID).Error
}

// cleanupPreviousDrafts 删除本任务（task_id）历史尝试遗留的草稿文章及其配图，
// 已发布文章不受影响，保证任务重试的幂等性。
func (p *Pipeline) cleanupPreviousDrafts(ctx context.Context, taskID uint, prog *progressReporter) {
	var ids []uint
	err := p.db.WithContext(ctx).Model(&model.Article{}).
		Joins("JOIN repo_analysis ON repo_analysis.id = article.repo_analysis_id").
		Where("repo_analysis.task_id = ? AND article.status = ?", taskID, model.ArticleStatusDraft).
		Pluck("article.id", &ids).Error
	if err != nil || len(ids) == 0 {
		return
	}
	p.db.WithContext(ctx).Where("article_id IN ?", ids).Delete(&model.SvgAsset{})
	if err := p.db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.Article{}).Error; err != nil {
		p.log.Warn("清理历史草稿失败", zap.Uint("task_id", taskID), zap.Error(err))
		return
	}
	prog.logf("已清理历史草稿文章 %d 篇", len(ids))
}

// uniqueSlug 归一化 slug 并保证库内唯一，冲突时追加短随机后缀。
func (p *Pipeline) uniqueSlug(ctx context.Context, preferred string) string {
	base := sanitizeSlug(preferred)
	if base == "" {
		base = "article"
	}
	slug := base
	for {
		var count int64
		if err := p.db.WithContext(ctx).Model(&model.Article{}).
			Where("slug = ?", slug).Count(&count).Error; err != nil {
			return slug // 查询失败时交给唯一索引兜底
		}
		if count == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%s", base, randomSuffix())
	}
}

// sanitizeSlug 归一化为 kebab-case：字母/数字保留（含中文，避免信息丢失），
// 下划线、空格等折叠为连字符。
func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 96 {
		out = strings.Trim(string([]rune(out)[:96]), "-")
	}
	return out
}

// randomSuffix 生成 6 位十六进制随机后缀。
func randomSuffix() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// fail 记录并落库任务失败；因服务关闭（ctx 取消）导致时不标记失败，
// 返回原错误交给 MQ 重新投递。
func (p *Pipeline) fail(ctx context.Context, prog *progressReporter, taskID uint, err error) error {
	if ctx.Err() != nil {
		prog.logf("任务中断：%v", err)
		return err
	}
	p.log.Error("任务执行失败", zap.Uint("task_id", taskID), zap.Error(err))
	prog.logf("任务失败：%v", err)
	prog.step(0, "失败", truncate(err.Error(), 200))
	p.updateTask(ctx, taskID, map[string]any{
		"status":  model.TaskStatusFailed,
		"message": "任务失败",
		"error":   truncate(err.Error(), 1000),
	})
	return err
}

// updateTask 更新任务行（忽略错误，进度更新不阻断主流程）。
func (p *Pipeline) updateTask(ctx context.Context, taskID uint, updates map[string]any) {
	if err := p.db.WithContext(ctx).Model(&model.GenTask{}).
		Where("id = ?", taskID).Updates(updates).Error; err != nil {
		p.log.Warn("更新任务状态失败", zap.Uint("task_id", taskID), zap.Error(err))
	}
}

// countWords 统计字数：CJK 字符按字计，连续拉丁字母/数字按词计。
func countWords(s string) int {
	n := 0
	inWord := false
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			n++
			inWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				n++
				inWord = true
			}
		default:
			inWord = false
		}
	}
	return n
}

// jsonify 序列化为 model.JSON（存库用），失败返回空对象。
func jsonify(v any) model.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		return model.JSON("{}")
	}
	return model.JSON(b)
}

func isDuplicateErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate")
}

func truncate(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n]) + "..."
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstNonEmptySlice(vals ...[]string) []string {
	for _, v := range vals {
		if len(v) > 0 {
			return v
		}
	}
	return nil
}
