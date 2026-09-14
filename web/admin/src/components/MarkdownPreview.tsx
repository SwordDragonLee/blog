import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Figure } from '../types';
import { figureUrl } from '../types';

/**
 * 将正文中的 {{figure:xxx}} 占位替换为配图 Markdown 图片；
 * 无法匹配到配图时降级为文字标记（预览中不会渲染任何原始 HTML，天然安全）。
 */
function resolveFigures(md: string, figures: Figure[]): string {
  return md.replace(/\{\{figure:([^}]+)\}\}/g, (_all, key: string) => {
    const fig = figures.find(
      (f) => String(f.id) === key || f.placeholder === key,
    );
    if (fig) {
      return `![${fig.title}](${figureUrl(fig.id)})`;
    }
    return `**【配图：${key}】**`;
  });
}

export function MarkdownPreview({
  content,
  figures = [],
}: {
  content: string;
  figures?: Figure[];
}) {
  return (
    <div className="preview-area">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>
        {resolveFigures(content || '', figures)}
      </ReactMarkdown>
    </div>
  );
}
