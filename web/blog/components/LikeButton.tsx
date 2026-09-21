"use client";

import { useEffect, useState } from "react";
import { likeArticle } from "@/lib/api";
import HeartIcon from "@/components/HeartIcon";

/** localStorage 记忆已点赞文章：刷新后仍展示已赞态（服务端仍按 IP 去重兜底） */
function likedKey(slug: string) {
  return `blog:liked:${slug}`;
}

/**
 * 文章点赞按钮：乐观 +1，以服务端返回计数为准；
 * 已赞或请求中不可重复点击。
 */
export default function LikeButton({
  slug,
  initialCount,
}: {
  slug: string;
  initialCount: number;
}) {
  const [count, setCount] = useState(initialCount);
  const [liked, setLiked] = useState(false);
  const [pending, setPending] = useState(false);

  // 水合后再读 localStorage，避免 SSR/CSR 渲染不一致
  useEffect(() => {
    try {
      if (window.localStorage.getItem(likedKey(slug)) === "1") {
        setLiked(true);
      }
    } catch {
      // localStorage 不可用（如隐私模式）时忽略，仅影响记忆样式
    }
  }, [slug]);

  async function handleClick() {
    if (liked || pending) return;
    setPending(true);
    setCount((c) => c + 1); // 乐观更新，失败回滚
    const result = await likeArticle(slug);
    if (result) {
      setCount(result.like_count);
      setLiked(true);
      try {
        window.localStorage.setItem(likedKey(slug), "1");
      } catch {
        // 同上，忽略
      }
    } else {
      setCount((c) => Math.max(initialCount, c - 1));
    }
    setPending(false);
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      aria-pressed={liked}
      className={`inline-flex items-center gap-2 rounded-full border px-5 py-2 text-sm font-medium transition ${
        liked
          ? "cursor-default border-rose-200 bg-rose-50 text-rose-600"
          : "border-slate-200 bg-white text-slate-600 hover:border-rose-300 hover:text-rose-600 hover:shadow-sm"
      }`}
    >
      <HeartIcon filled={liked} />
      <span>{count > 0 ? count : "点赞"}</span>
    </button>
  );
}
