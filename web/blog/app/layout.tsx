import type { Metadata } from "next";
import Link from "next/link";
import AiChat from "@/components/AiChat";
import "./globals.css";
import "highlight.js/styles/github-dark.css";

export const metadata: Metadata = {
  title: {
    default: "AI 技术观察",
    template: "%s | AI 技术观察",
  },
  description: "由 AI 博客生成平台自动产出的技术观察与工程实践文章",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN">
      <body className="antialiased">
        <div className="flex min-h-screen flex-col">
          <header className="sticky top-0 z-20 border-b border-slate-200/80 bg-white/85 backdrop-blur">
            <div className="mx-auto flex w-full max-w-5xl items-center justify-between px-4 py-3">
              <Link href="/" className="flex items-center gap-2.5">
                <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-blue-600 to-cyan-500 text-sm font-bold text-white shadow-sm">
                  AI
                </span>
                <span className="flex flex-col leading-tight">
                  <span className="text-[17px] font-bold tracking-tight text-slate-900">
                    AI 技术观察
                  </span>
                  <span className="text-xs text-slate-500">
                    洞察 AI 前沿与工程实践
                  </span>
                </span>
              </Link>
              <nav className="text-sm font-medium text-slate-600">
                <Link
                  href="/"
                  className="rounded-lg px-3 py-1.5 transition hover:bg-slate-100 hover:text-blue-600"
                >
                  文章
                </Link>
              </nav>
            </div>
          </header>

          {children}

          {/* 全站悬浮 AI 问答（基于站内文章的 RAG 检索回答） */}
          <AiChat />

          <footer className="mt-14 border-t border-slate-200 bg-white">
            <div className="mx-auto w-full max-w-5xl px-4 py-6 text-center text-sm text-slate-500">
              Powered by AI 博客生成平台
            </div>
          </footer>
        </div>
      </body>
    </html>
  );
}
