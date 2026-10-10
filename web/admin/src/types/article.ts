// 文章与配图（与后端 model.Article / model.SvgAsset 对齐，JSON tag 同名）

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

export const ARTICLE_STATUS_TEXT: Record<string, string> = {
  draft: '待审核',
  published: '已发布',
  offline: '已下线',
};

// 前台配图访问地址（经 vite proxy 转发到 Go 后端）
export function figureUrl(id: number): string {
  return `/api/v1/portal/figures/${id}`;
}
