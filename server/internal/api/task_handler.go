package api

import (
	"blog/server/internal/httputil"
	"blog/server/internal/resp"
	"blog/server/internal/service"

	"github.com/gin-gonic/gin"
)

// TaskHandler 生成任务管理接口。
type TaskHandler struct {
	svc *service.TaskService
}

// NewTaskHandler 创建任务 handler。
func NewTaskHandler(svc *service.TaskService) *TaskHandler { return &TaskHandler{svc: svc} }

// Create POST /api/v1/tasks：创建生成任务并投递 MQ。
//
//	@Summary 创建生成任务
//	@Description 克隆公开 Git 仓库 → 静态分析 → LLM 生成 4 篇中文文章（含 SVG 配图），落库为草稿。
//	@Tags 任务
//	@Security BearerAuth
//	@Accept json
//	@Produce json
//	@Param body body createTaskRequest true "Git 仓库地址"
//	@Success 200 {object} resp.Envelope{data=model.GenTask}
//	@Router /tasks [post]
func (h *TaskHandler) Create(c *gin.Context) {
	var req createTaskRequest
	if !httputil.BindJSON(c, &req) {
		return
	}
	t, err := h.svc.Create(c.Request.Context(), req.GitURL)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, t)
}

// List GET /api/v1/tasks：任务分页列表。
//
//	@Summary 任务分页列表
//	@Tags 任务
//	@Security BearerAuth
//	@Produce json
//	@Param page query int false "页码，默认 1"
//	@Param page_size query int false "每页条数，默认 10，上限 100"
//	@Success 200 {object} resp.Envelope{data=service.PageData{items=[]model.GenTask}}
//	@Router /tasks [get]
func (h *TaskHandler) List(c *gin.Context) {
	page, pageSize := httputil.PageParams(c)
	pd, err := h.svc.List(c.Request.Context(), page, pageSize)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, pd)
}

// Get GET /api/v1/tasks/:id：任务详情（含 Redis 实时进度与日志，管理端每 2 秒轮询用）。
//
//	@Summary 任务详情
//	@Tags 任务
//	@Security BearerAuth
//	@Produce json
//	@Param id path int true "任务 ID"
//	@Success 200 {object} resp.Envelope{data=service.TaskDetail}
//	@Router /tasks/{id} [get]
func (h *TaskHandler) Get(c *gin.Context) {
	id, okID := httputil.PathUint(c, "id")
	if !okID {
		return
	}
	detail, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, detail)
}

// Retry POST /api/v1/tasks/:id/retry：失败任务重跑（重新入队）。
//
//	@Summary 重跑失败任务
//	@Tags 任务
//	@Security BearerAuth
//	@Produce json
//	@Param id path int true "任务 ID"
//	@Success 200 {object} resp.Envelope{data=model.GenTask}
//	@Router /tasks/{id}/retry [post]
func (h *TaskHandler) Retry(c *gin.Context) {
	id, okID := httputil.PathUint(c, "id")
	if !okID {
		return
	}
	t, err := h.svc.Retry(c.Request.Context(), id)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, t)
}

// Cancel POST /api/v1/tasks/:id/cancel：取消排队中/运行中的任务。
//
//	@Summary 取消任务
//	@Tags 任务
//	@Security BearerAuth
//	@Produce json
//	@Param id path int true "任务 ID"
//	@Success 200 {object} resp.Envelope{data=model.GenTask}
//	@Router /tasks/{id}/cancel [post]
func (h *TaskHandler) Cancel(c *gin.Context) {
	id, okID := httputil.PathUint(c, "id")
	if !okID {
		return
	}
	t, err := h.svc.Cancel(c.Request.Context(), id)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, t)
}

// Delete DELETE /api/v1/tasks/:id：删除任务及其衍生数据（分析记录、草稿文章、配图）。
//
//	@Summary 删除任务
//	@Tags 任务
//	@Security BearerAuth
//	@Produce json
//	@Param id path int true "任务 ID"
//	@Success 200 {object} resp.Envelope
//	@Router /tasks/{id} [delete]
func (h *TaskHandler) Delete(c *gin.Context) {
	id, okID := httputil.PathUint(c, "id")
	if !okID {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, nil)
}

type createTaskRequest struct {
	GitURL string `json:"git_url"`
}
