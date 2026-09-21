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

// Create POST /tasks：创建生成任务并投递 MQ。
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

// List GET /tasks：任务分页列表。
func (h *TaskHandler) List(c *gin.Context) {
	page, pageSize := httputil.PageParams(c)
	pd, err := h.svc.List(c.Request.Context(), page, pageSize)
	if err != nil {
		resp.FailWith(c, err)
		return
	}
	resp.OK(c, pd)
}

// Get GET /tasks/:id：任务详情（含 Redis 实时进度与日志，轮询用）。
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

// Retry POST /tasks/:id/retry：失败任务重跑（重新入队）。
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

// Cancel POST /tasks/:id/cancel：取消排队中/运行中的任务。
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

// Delete DELETE /tasks/:id：删除任务及其衍生数据（分析记录、草稿文章、配图）。
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
