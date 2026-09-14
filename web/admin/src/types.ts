// 与后端 model 包对齐的类型定义（JSON tag 同名）

export interface AdminUser {
  id: number;
  username: string;
  created_at?: string;
}

export interface LoginResp {
  token: string;
  user: AdminUser;
}

// 生成任务状态
export type TaskStatus = 'pending' | 'running' | 'success' | 'failed';

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
}

// 文章状态：draft（待审核）→ publish → published → offline 回到 draft
export type ArticleStatus = 'draft' | 'published' | 'offline';

export interface Article {
  id: number;
  repo_analysis_id: number;
  title: string;
  slug: string;
  summary: string;
  content_md?: string;
  word_count: number;
  tags: string[];
  status: ArticleStatus;
  sort_order: number;
  cover_asset_id?: number | null;
  published_at?: string | null;
  created_at: string;
  updated_at: string;
}

// SVG 配图（svg_asset），正文通过 {{figure:xxx}} 占位
export interface Figure {
  id: number;
  article_id: number;
  placeholder: string;
  kind: 'cover' | 'architecture' | 'flow' | 'compare' | 'timeline' | string;
  title: string;
  created_at: string;
}

export interface ArticleDetail extends Article {
  figures?: Figure[];
}

export interface ArticleUpdatePayload {
  title: string;
  summary: string;
  content_md: string;
  tags: string[];
}

// 统一分页响应 data
export interface PageData<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export const ARTICLE_STATUS_TEXT: Record<string, string> = {
  draft: '待审核',
  published: '已发布',
  offline: '已下线',
};

export const TASK_STATUS_TEXT: Record<string, string> = {
  pending: '等待中',
  running: '运行中',
  success: '成功',
  failed: '失败',
};

// 前台配图访问地址（经 vite proxy 转发到 Go 后端）
export function figureUrl(id: number): string {
  return `/api/v1/portal/figures/${id}`;
}
