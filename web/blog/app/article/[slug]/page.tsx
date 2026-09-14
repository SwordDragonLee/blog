import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import {
  figureUrl,
  formatDate,
  formatWordCount,
  getPortalArticle,
  inlineFigures,
  normalizeTags,
} from "@/lib/api";

/** ISR：60s 重新验证 */
export const revalidate = 60;

const COVER_GRADIENT = "from-blue-600 via-sky-500 to-cyan-500";

type Props = { params: Promise<{ slug: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params;
  const article = await getPortalArticle(slug);
  if (!article) {
    return { title: "文章不存在" };
  }
  return {
    title: article.title,
    description: article.summary || undefined,
  };
}

export default async function ArticlePage({ params }: Props) {
  const { slug } = await params;
  const article = await getPortalArticle(slug);
  if (!article) notFound();

  // 先把 {{figure:xxx}} 占位符替换成图片，再交给 react-markdown 渲染
  const markdown = inlineFigures(article.content_md ?? "", article.figures ?? []);
  const tags = normalizeTags(article.tags);
  const date = formatDate(article.published_at);
  const words = formatWordCount(article.word_count);
  const cover = figureUrl(article.cover?.id);

  return (
    <main className="mx-auto w-full max-w-3xl flex-1 px-4 py-8 sm:py-10">
      <div className="mb-5">
        <Link
          href="/"
          className="text-sm text-slate-500 transition hover:text-blue-600"
        >
          ← 返回文章列表
        </Link>
      </div>

      <article className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
        {cover ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={cover}
            alt={article.cover?.title || article.title}
            className="max-h-96 w-full object-cover"
          />
        ) : (
          <div className={`h-40 w-full bg-gradient-to-br ${COVER_GRADIENT} sm:h-48`} />
        )}

        <div className="px-5 py-8 sm:px-10 sm:py-10">
          <h1 className="text-2xl font-bold leading-tight text-slate-900 sm:text-[32px]">
            {article.title}
          </h1>

          {article.summary && (
            <p className="mt-4 rounded-xl bg-slate-50 p-4 text-sm leading-relaxed text-slate-600">
              {article.summary}
            </p>
          )}

          <div className="mt-5 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-slate-500">
            {date && <time dateTime={date}>{date}</time>}
            {words && (
              <>
                <span className="text-slate-300">·</span>
                <span>{words}</span>
              </>
            )}
            {tags.length > 0 && (
              <>
                <span className="text-slate-300">·</span>
                <span className="flex flex-wrap gap-1.5">
                  {tags.map((tag) => (
                    <Link
                      key={tag}
                      href={`/?tag=${encodeURIComponent(tag)}`}
                      className="rounded-full bg-slate-100 px-2.5 py-0.5 text-xs text-slate-600 transition hover:bg-blue-50 hover:text-blue-600"
                    >
                      {tag}
                    </Link>
                  ))}
                </span>
              </>
            )}
          </div>

          <hr className="my-7 border-slate-100" />

          <div className="prose-blog">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              rehypePlugins={[rehypeHighlight]}
            >
              {markdown}
            </ReactMarkdown>
          </div>
        </div>
      </article>

      <div className="mt-6 text-center">
        <Link
          href="/"
          className="inline-block rounded-lg border border-slate-200 bg-white px-5 py-2.5 text-sm text-slate-600 shadow-sm transition hover:border-blue-300 hover:text-blue-600"
        >
          ← 返回文章列表
        </Link>
      </div>
    </main>
  );
}
