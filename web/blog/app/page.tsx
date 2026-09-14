import Link from "next/link";
import ArticleCard from "@/components/ArticleCard";
import { collectTags, getPortalArticles } from "@/lib/api";

/** ISR：60s 重新验证 */
export const revalidate = 60;

const PAGE_SIZE = 10;

type SearchParams = { [key: string]: string | string[] | undefined };

function firstString(v: string | string[] | undefined): string | undefined {
  return Array.isArray(v) ? v[0] : v;
}

export default async function HomePage({
  searchParams,
}: {
  searchParams: Promise<SearchParams>;
}) {
  const sp = await searchParams;
  const page = Math.max(1, Number.parseInt(firstString(sp.page) ?? "1", 10) || 1);
  const tag = firstString(sp.tag)?.trim() || undefined;

  // 标签列表由全量已发布文章聚合去重；正文分页按 page/tag 查询
  const [listForTags, list] = await Promise.all([
    getPortalArticles({ page: 1, pageSize: 100 }),
    getPortalArticles({ page, pageSize: PAGE_SIZE, tag }),
  ]);

  const tags = collectTags(listForTags?.items ?? []);
  const items = list?.items ?? [];
  const total = list?.total ?? 0;
  const pageSize = list?.page_size || PAGE_SIZE;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const hrefWith = (next: { page?: number; tag?: string | null }) => {
    const qs = new URLSearchParams();
    const p = next.page ?? page;
    const t = next.tag === null ? undefined : (next.tag ?? tag);
    if (p > 1) qs.set("page", String(p));
    if (t) qs.set("tag", t);
    const s = qs.toString();
    return s ? `/?${s}` : "/";
  };

  const chip = "rounded-full px-3 py-1 text-xs font-medium transition";
  const chipIdle = `${chip} border border-slate-200 bg-white text-slate-600 shadow-sm hover:border-blue-300 hover:text-blue-600`;
  const chipActive = `${chip} border border-blue-600 bg-blue-600 text-white shadow-sm`;

  const pageBtn =
    "rounded-lg border px-4 py-2 text-sm font-medium shadow-sm transition";
  const pageBtnOn = `${pageBtn} border-slate-200 bg-white text-slate-700 hover:border-blue-300 hover:text-blue-600`;
  const pageBtnOff = `${pageBtn} border-slate-100 bg-slate-50 text-slate-400`;

  return (
    <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-8 sm:py-10">
      <div className="mb-7">
        <h1 className="text-2xl font-bold text-slate-900 sm:text-3xl">
          最新文章
        </h1>
        <p className="mt-1.5 text-sm text-slate-500">
          共 {total} 篇已发布文章{tag ? ` · 标签「${tag}」` : ""}
        </p>

        {tags.length > 0 && (
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <Link href={hrefWith({ page: 1, tag: null })} className={tag ? chipIdle : chipActive}>
              全部
            </Link>
            {tags.map((t) => (
              <Link key={t} href={hrefWith({ page: 1, tag: t })} className={t === tag ? chipActive : chipIdle}>
                {t}
              </Link>
            ))}
          </div>
        )}
      </div>

      {items.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-300 bg-white px-6 py-20 text-center">
          <p className="text-lg font-semibold text-slate-700">暂无文章</p>
          <p className="mt-2 text-sm text-slate-500">
            {tag
              ? `没有「${tag}」标签下的文章，换一个标签看看。`
              : "内容正在生成中，请稍后再来。"}
          </p>
          {tag && (
            <Link
              href="/"
              className="mt-6 inline-block rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm text-slate-600 shadow-sm transition hover:border-blue-300 hover:text-blue-600"
            >
              查看全部文章
            </Link>
          )}
        </div>
      ) : (
        <div className="grid gap-6 sm:grid-cols-2">
          {items.map((article) => (
            <ArticleCard key={article.id} article={article} />
          ))}
        </div>
      )}

      {totalPages > 1 && (
        <nav className="mt-10 flex items-center justify-between" aria-label="分页">
          {page > 1 ? (
            <Link href={hrefWith({ page: page - 1 })} className={pageBtnOn}>
              ← 上一页
            </Link>
          ) : (
            <span className={pageBtnOff}>← 上一页</span>
          )}
          <span className="text-sm text-slate-500">
            第 {page} / {totalPages} 页
          </span>
          {page < totalPages ? (
            <Link href={hrefWith({ page: page + 1 })} className={pageBtnOn}>
              下一页 →
            </Link>
          ) : (
            <span className={pageBtnOff}>下一页 →</span>
          )}
        </nav>
      )}
    </main>
  );
}
