// 生成任务（与后端 model.GenTask 对齐，JSON tag 同名）

// 生成任务状态
export type TaskStatus = 'pending' | 'running' | 'success' | 'failed' | 'canceled';

export interface GenTask {
  id: number;
  git_url: string;
  status: TaskStatus;
  progress: number; // 0-100
  step: string;
  message: string;
  error: string;
  repo_analysis_id?: number | null;
  created_at: string;
  updated_at: string;
  /** 详情接口返回的执行日志（Redis 保留最近 200 条，7 天过期）；列表接口无此字段 */
  logs?: string[];
}

export const TASK_STATUS_TEXT: Record<string, string> = {
  pending: '等待中',
  running: '运行中',
  success: '成功',
  failed: '失败',
  canceled: '已取消',
};
