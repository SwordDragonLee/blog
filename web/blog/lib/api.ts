/**
 * 服务端 API 访问封装。
 *
 * - RSC / 服务组件里无法用相对路径发起 fetch，这里统一用 API_BASE 拼绝对地址；
 *   可通过环境变量 API_BASE 覆盖，默认 http://localhost:8080。
 * - 客户端代码与 <img> 图片则走 next.config.ts 里的 rewrites，用相对路径 /api/v1/...。
 * - 后端统一响应结构：{ code: 0, message: "ok", data: ... }。
 */

export const API_BASE = process.env.API_BASE ?? "http://localhost:8080";

/** ISR / 数据缓存时长（秒） */
export const REVALIDATE_SECONDS = 60;

export interface PortalFigure {
  id: number;
  placeholder?: string | null;
  kind?: string | null;
  title?: string | null;
}

export interface PortalCover {
  id: number;
  kind?: string | null;
  title?: string | null;
}

export interface ArticleListItem {
  id: number;
  title: string;
  slug: string;
  summary?: string | null;
  /** 后端为 JSON 数组，这里宽松处理 */
  tags?: unknown;
  word_count?: number | null;
  status?: string | null;
  cover_asset_id?: number | null;
  published_at?: string | null;
}

export interface ArticleDetail {
  id: number;
  title: string;
  slug: string;
  summary?: string | null;
  content_md?: string | null;
  tags?: unknown;
  word_count?: number | null;
  published_at?: string | null;
  cover?: PortalCover | null;
  figures?: PortalFigure[];
}

export interface Paged<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

interface Envelope<T> {
  code: number;
  message: string;
  data: T;
}

/** 配图（含封面）统一走该相对路径，经 next rewrites 代理到 Go 后端 */
export function figureUrl(id: number | null | undefined): string | null {
  if (id === null || id === undefined) return null;
  return `/api/v1/portal/figures/${id}`;
}

async function request<T>(path: string, revalidate = REVALIDATE_SECONDS): Promise<T | null> {
  try {
    const res = await fetch(`${API_BASE}${path}`, {
      // ISR：服务端缓存 60s，详情页/首页数据 60s 内复用
      next: { revalidate },
      headers: { accept: "application/json" },
    });
    if (!res.ok) return null;
    const body = (await res.json()) as Envelope<T> | null;
    if (!body || typeof body.code !== "number" || body.code !== 0) return null;
    return body.data;
  } catch {
    // 后端不可用时不阻塞页面渲染，由调用方展示兜底 UI
    return null;
  }
}

export async function getPortalArticles(
  opts: { page?: number; pageSize?: number; tag?: string } = {},
): Promise<Paged<ArticleListItem> | null> {
  const page = opts.page ?? 1;
  const pageSize = opts.pageSize ?? 10;
  const qs = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  if (opts.tag) qs.set("tag", opts.tag);
  return request<Paged<ArticleListItem>>(`/api/v1/portal/articles?${qs.toString()}`);
}

export async function getPortalArticle(slug: string): Promise<ArticleDetail | null> {
  return request<ArticleDetail>(`/api/v1/portal/articles/${encodeURIComponent(slug)}`);
}

/** tags 后端是 JSON 数组（也可能被序列化成字符串），统一归一化为 string[] */
export function normalizeTags(raw: unknown): string[] {
  let arr: unknown[] = [];
  if (Array.isArray(raw)) {
    arr = raw;
  } else if (typeof raw === "string" && raw.trim()) {
    try {
      const parsed: unknown = JSON.parse(raw);
      arr = Array.isArray(parsed) ? parsed : [String(parsed)];
    } catch {
      arr = raw.split(/[,，;；\s]+/);
    }
  }
  return arr.map((t) => String(t).trim()).filter(Boolean);
}

/** 汇总去重所有文章的标签 */
export function collectTags(items: { tags?: unknown }[]): string[] {
  const seen = new Set<string>();
  for (const item of items) {
    for (const tag of normalizeTags(item.tags)) {
      seen.add(tag);
    }
  }
  return [...seen];
}

/** 发布日期格式化为 YYYY-MM-DD */
export function formatDate(iso?: string | null): string | null {
  if (!iso) return null;
  const m = /^(\d{4}-\d{2}-\d{2})/.exec(iso);
  if (m) return m[1];
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString().slice(0, 10);
}

/** 字数显示：约 1,234 字（避免 toLocaleString 在 SSR/CSR 间差异） */
export function formatWordCount(n?: number | null): string | null {
  if (typeof n !== "number" || !Number.isFinite(n) || n <= 0) return null;
  const withComma = Math.round(n)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return `约 ${withComma} 字`;
}

/**
 * 把正文中的 {{figure:xxx}} 占位符替换为 Markdown 图片，
 * 供 react-markdown 渲染；xxx 为 figures[].placeholder，映射到其素材 id。
 */
export function inlineFigures(contentMd: string, figures?: PortalFigure[]): string {
  const md = contentMd ?? "";
  if (!md || !figures || figures.length === 0) return md;
  const byPlaceholder = new Map(
    figures.filter((f) => f && f.placeholder && f.id !== null && f.id !== undefined)
      .map((f) => [f.placeholder as string, f]),
  );
  if (byPlaceholder.size === 0) return md;
  return md.replace(/\{\{figure:([^}]+)\}\}/g, (raw, key: string) => {
    const fig = byPlaceholder.get(key.trim());
    if (!fig) return raw;
    const alt = (fig.title ?? "").replace(/[[\]]/g, "").trim() || fig.placeholder || "配图";
    return `![${alt}](${figureUrl(fig.id)})`;
  });
}
