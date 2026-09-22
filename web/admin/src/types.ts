// 与后端 model 包对齐的类型定义（JSON tag 同名）

export interface AdminUser {
  id: number;
  username: string;
  email?: string;
  created_at?: string;
}

export interface LoginResp {
  token: string;
  user: AdminUser;
}

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
  like_count?: number;
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
  canceled: '已取消',
};

// 前台配图访问地址（经 vite proxy 转发到 Go 后端）
export function figureUrl(id: number): string {
  return `/api/v1/portal/figures/${id}`;
}

// RAG 向量索引总览（按文章分组的向量块统计）
export interface RagIndexStat {
  slug: string;
  title: string;
  chunks: number;
}

export interface RagIndexOverview {
  articles: RagIndexStat[];
  total_chunks: number;
}

// 检索测试命中（pass = 得分达到阈值，真实问答会采纳）
export interface RagProbeHit {
  score: number;
  slug: string;
  title: string;
  snippet: string;
  pass: boolean;
}

// 问答无命中问题：选题回流（同一归一化键聚合，hits 为提问次数）
export interface AskUnanswered {
  id: number;
  question: string;
  normalized_key: string;
  hits: number;
  first_seen_at: string;
  last_seen_at: string;
}
