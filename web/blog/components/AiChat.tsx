"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";

/** 一条回答的引用出处（与后端 Citation 对齐） */
interface Citation {
  title: string;
  slug: string;
  score: number;
  snippet: string;
}

/** 会话消息；citations 仅助手消息携带 */
interface ChatMsg {
  role: "user" | "assistant";
  content: string;
  citations?: Citation[];
  /** 请求失败标记：气泡内展示错误文案 */
  failed?: boolean;
}

/** 携带最近 3 轮对话作为上下文（6 条消息） */
const HISTORY_TURNS = 3;

// dev 下直连 Go 后端（next rewrites 代理会对 SSE 做 gzip 缓冲，导致回答憋到结束一次性输出）；
// 生产不设 NEXT_PUBLIC_API_BASE，仍走同源 /api 由 nginx 直连后端（带 SSE 专项配置）。
const API_ORIGIN = process.env.NEXT_PUBLIC_API_BASE ?? "";

/**
 * 前台悬浮 AI 问答：基于站内已发布文章做检索增强回答（RAG），
 * 走 SSE 流式接口 POST /api/v1/portal/ask（dev 直连后端，生产经 nginx）。
 */
export default function AiChat() {
  const [open, setOpen] = useState(false);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const listRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // 新增量文本到达时贴底滚动
  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages, open]);

  // 打开面板自动聚焦输入框
  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  /** 组装请求历史：取当前提问之前最近 3 轮（user/assistant 成对） */
  function buildHistory(): { role: string; content: string }[] {
    const done = messages.filter((m) => !m.failed);
    return done
      .slice(Math.max(0, done.length - HISTORY_TURNS * 2))
      .map((m) => ({ role: m.role, content: m.content }));
  }

  async function send() {
    const question = input.trim();
    if (!question || sending) return;
    setInput("");
    setSending(true);
    const history = buildHistory();
    // 先落两条消息：用户提问 + 占位的助手回答（随后流式填充）
    setMessages((prev) => [
      ...prev,
      { role: "user", content: question },
      { role: "assistant", content: "" },
    ]);

    try {
      const res = await fetch(`${API_ORIGIN}/api/v1/portal/ask`, {
        method: "POST",
        headers: { "Content-Type": "application/json", accept: "text/event-stream" },
        body: JSON.stringify({ question, history }),
      });
      // 非 200：resp 统一信封（限流 429 / 未启用 503 等），直接展示 message
      if (!res.ok || !res.body) {
        let msg = "服务暂时不可用，请稍后再试";
        try {
          const body = await res.json();
          if (body?.message) msg = body.message;
        } catch {
          // 非 JSON 响应时用默认文案
        }
        markFailed(msg);
        return;
      }

      // 解析 SSE：按空行分帧，帧内 event: X + data: {json}
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      // eslint-disable-next-line no-constant-condition
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let sep: number;
        while ((sep = buffer.indexOf("\n\n")) >= 0) {
          const frame = buffer.slice(0, sep);
          buffer = buffer.slice(sep + 2);
          handleFrame(frame);
        }
      }
    } catch {
      markFailed("网络异常，请稍后重试");
    } finally {
      setSending(false);
    }
  }

  /** 处理单个 SSE 帧：按事件类型更新占位的助手消息 */
  function handleFrame(frame: string) {
    let event = "message";
    let data = "";
    for (const line of frame.split("\n")) {
      if (line.startsWith("event:")) event = line.slice(6).trim();
      else if (line.startsWith("data:")) data += line.slice(5).trim();
    }
    if (!data) return;
    let payload: unknown;
    try {
      payload = JSON.parse(data);
    } catch {
      return;
    }

    if (event === "delta") {
      const text = (payload as { text?: string }).text ?? "";
      setMessages((prev) => {
        const next = [...prev];
        const last = next[next.length - 1];
        if (last?.role === "assistant") {
          next[next.length - 1] = { ...last, content: last.content + text };
        }
        return next;
      });
    } else if (event === "citations") {
      const items = Array.isArray(payload) ? (payload as Citation[]) : [];
      setMessages((prev) => {
        const next = [...prev];
        const last = next[next.length - 1];
        if (last?.role === "assistant") {
          next[next.length - 1] = { ...last, citations: items };
        }
        return next;
      });
    } else if (event === "error") {
      const msg = (payload as { message?: string }).message ?? "回答生成失败，请稍后重试";
      markFailed(msg);
    }
    // done：流结束，占位消息已是最终内容，无需处理
  }

  /** 把最后一条助手消息标记为失败并填入错误文案 */
  function markFailed(message: string) {
    setMessages((prev) => {
      const next = [...prev];
      const last = next[next.length - 1];
      if (last?.role === "assistant") {
        next[next.length - 1] = { ...last, content: message, failed: true };
      }
      return next;
    });
  }

  return (
    <>
      {/* 悬浮入口：右下角圆形按钮 */}
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={open ? "关闭 AI 问答" : "打开 AI 问答"}
        className="fixed bottom-5 right-4 z-40 flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br from-blue-600 to-cyan-500 text-white shadow-lg shadow-blue-500/30 transition hover:scale-105 hover:shadow-xl sm:right-6"
      >
        {open ? (
          <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2">
            <path strokeLinecap="round" d="M6 6l12 12M18 6L6 18" />
          </svg>
        ) : (
          <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M8 10h8M8 14h5M21 12a9 9 0 11-4.2-7.6L21 3l-1.2 4.2A8.96 8.96 0 0121 12z"
            />
          </svg>
        )}
      </button>

      {open && (
        <div className="fixed bottom-24 right-4 z-40 flex h-[560px] max-h-[calc(100vh-8rem)] w-[380px] max-w-[calc(100vw-2rem)] flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl sm:right-6">
          {/* 头部 */}
          <div className="flex items-center justify-between border-b border-slate-100 bg-gradient-to-r from-blue-600 to-cyan-500 px-4 py-3 text-white">
            <div>
              <div className="text-sm font-bold">AI 技术问答</div>
              <div className="text-xs text-blue-100">基于本站文章内容回答</div>
            </div>
            <button
              type="button"
              onClick={() => setOpen(false)}
              aria-label="关闭"
              className="rounded-lg p-1.5 transition hover:bg-white/15"
            >
              <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2">
                <path strokeLinecap="round" d="M6 6l12 12M18 6L6 18" />
              </svg>
            </button>
          </div>

          {/* 消息列表 */}
          <div ref={listRef} className="flex-1 space-y-4 overflow-y-auto bg-slate-50/60 px-4 py-4">
            {messages.length === 0 && (
              <div className="mt-10 text-center text-sm text-slate-400">
                <div className="mb-2 text-3xl">🤖</div>
                向我提问本站文章相关的技术问题
                <div className="mt-1 text-xs">例如：这个项目的任务队列是怎么实现的？</div>
              </div>
            )}
            {messages.map((m, i) => (
              <div key={i} className={`flex ${m.role === "user" ? "justify-end" : "justify-start"}`}>
                <div
                  className={`max-w-[85%] rounded-2xl px-3.5 py-2.5 ${
                    m.role === "user"
                      ? "rounded-br-md bg-blue-600 text-white"
                      : m.failed
                        ? "rounded-bl-md border border-rose-200 bg-rose-50 text-rose-600"
                        : "rounded-bl-md border border-slate-200 bg-white text-slate-700 shadow-sm"
                  }`}
                >
                  {m.role === "assistant" && !m.failed ? (
                    m.content ? (
                      <div className="prose-chat">
                        <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
                          {m.content}
                        </ReactMarkdown>
                      </div>
                    ) : (
                      /* 流式开始前的等待动画 */
                      <div className="flex items-center gap-1 py-1" aria-label="正在思考">
                        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-slate-400 [animation-delay:0ms]" />
                        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-slate-400 [animation-delay:150ms]" />
                        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-slate-400 [animation-delay:300ms]" />
                      </div>
                    )
                  ) : (
                    <p className="whitespace-pre-wrap text-sm leading-relaxed">{m.content}</p>
                  )}

                  {/* 引用出处：序号与回答中的 [n] 标注对应 */}
                  {m.role === "assistant" && !m.failed && m.citations && m.citations.length > 0 && (
                    <div className="mt-2.5 border-t border-slate-100 pt-2">
                      <div className="mb-1.5 text-xs font-medium text-slate-400">资料来源</div>
                      <div className="space-y-1">
                        {m.citations.map((c, idx) => (
                          <Link
                            key={`${c.slug}-${idx}`}
                            href={`/article/${c.slug}`}
                            className="flex items-start gap-1.5 rounded-lg px-1.5 py-1 text-xs text-slate-600 transition hover:bg-blue-50 hover:text-blue-600"
                          >
                            <span className="mt-px flex h-4 w-4 shrink-0 items-center justify-center rounded bg-slate-100 text-[10px] font-semibold text-slate-500">
                              {idx + 1}
                            </span>
                            <span className="line-clamp-1">{c.title}</span>
                          </Link>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>

          {/* 输入区 */}
          <div className="border-t border-slate-100 bg-white px-3 py-2.5">
            <div className="flex items-end gap-2">
              <textarea
                ref={inputRef}
                rows={1}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  // Enter 发送，Shift+Enter 换行
                  if (e.key === "Enter" && !e.shiftKey) {
                    e.preventDefault();
                    send();
                  }
                }}
                placeholder="输入你的问题…"
                className="max-h-28 flex-1 resize-none rounded-xl border border-slate-200 px-3 py-2 text-sm text-slate-700 placeholder:text-slate-400 focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100"
              />
              <button
                type="button"
                onClick={send}
                disabled={sending || !input.trim()}
                aria-label="发送"
                className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-blue-600 text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-40"
              >
                <svg viewBox="0 0 24 24" className="h-4.5 w-4.5" fill="none" stroke="currentColor" strokeWidth="2">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 12h14M13 6l6 6-6 6" />
                </svg>
              </button>
            </div>
            <div className="mt-1.5 text-center text-[11px] text-slate-400">
              回答由 AI 基于站内文章生成，可能存在错漏
            </div>
          </div>
        </div>
      )}
    </>
  );
}
