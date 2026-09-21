import Link from "next/link";
import {
  figureUrl,
  formatDate,
  formatWordCount,
  normalizeTags,
  type ArticleListItem,
} from "@/lib/api";
import HeartIcon from "@/components/HeartIcon";

/** 无封面时的渐变占位（按文章 id 稳定选取，刷新不变） */
const COVER_GRADIENTS = [
  "from-blue-600 via-sky-500 to-cyan-400",
  "from-violet-600 via-purple-500 to-fuchsia-500",
  "from-emerald-600 via-teal-500 to-cyan-500",
  "from-orange-500 via-amber-500 to-yellow-400",
  "from-slate-800 via-slate-700 to-slate-500",
];

export default function ArticleCard({ article }: { article: ArticleListItem }) {
  const cover = figureUrl(article.cover_asset_id);
  const gradient =
    COVER_GRADIENTS[Math.abs(article.id) % COVER_GRADIENTS.length] ??
    COVER_GRADIENTS[0];
  const date = formatDate(article.published_at);
  const words = formatWordCount(article.word_count);
  const tags = normalizeTags(article.tags).slice(0, 4);

  return (
    <article className="group flex flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm transition duration-200 hover:-translate-y-0.5 hover:shadow-lg">
      <Link
        href={`/article/${encodeURIComponent(article.slug)}`}
        className="block h-44 overflow-hidden"
        aria-label={article.title}
      >
        {cover ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={cover}
            alt={article.title}
            loading="lazy"
            className="h-full w-full object-cover transition duration-300 group-hover:scale-[1.03]"
          />
        ) : (
          <div
            className={`flex h-full w-full items-center justify-center bg-gradient-to-br ${gradient} transition duration-300 group-hover:opacity-95`}
          >
            <span className="select-none text-3xl font-black tracking-[0.3em] text-white/90">
              AI
            </span>
          </div>
        )}
      </Link>

      <div className="flex flex-1 flex-col p-5">
        <h2 className="text-lg font-bold leading-snug text-slate-900">
          <Link
            href={`/article/${encodeURIComponent(article.slug)}`}
            className="transition hover:text-blue-600"
          >
            {article.title}
          </Link>
        </h2>

        <p className="mt-2 line-clamp-3 flex-1 text-sm leading-relaxed text-slate-600">
          {article.summary || "（暂无摘要）"}
        </p>

        {tags.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-1.5">
            {tags.map((tag) => (
              <span
                key={tag}
                className="rounded-full bg-slate-100 px-2.5 py-0.5 text-xs text-slate-600"
              >
                {tag}
              </span>
            ))}
          </div>
        )}

        <div className="mt-4 flex items-center justify-between border-t border-slate-100 pt-3 text-xs text-slate-400">
          <span>{date ?? "未发布"}</span>
          <span className="flex items-center gap-3">
            {words && <span>{words}</span>}
            {(article.like_count ?? 0) > 0 && (
              <span className="flex items-center gap-1">
                <HeartIcon className="h-3 w-3" />
                {article.like_count}
              </span>
            )}
          </span>
        </div>
      </div>
    </article>
  );
}
