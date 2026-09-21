"use client";

import { useEffect, useState } from "react";
import { QRCodeSVG } from "qrcode.react";

/**
 * 文章分享按钮：弹出文章链接二维码 + 复制链接。
 * 微信场景采用「扫码打开」模式：在微信内打开后可用微信自带菜单
 * 转发给好友或分享到朋友圈（网页无法直调微信分享，需认证公众号）。
 */

/**
 * 复制文本到剪贴板：优先用 Clipboard API（仅 HTTPS/localhost 等安全上下文可用），
 * 不可用或失败时降级为隐藏 textarea + execCommand（兼容 http 局域网 IP 访问）。
 */
async function copyText(text: string): Promise<boolean> {
  if (window.isSecureContext && navigator.clipboard) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // 权限被拒等情况，继续尝试降级方案
    }
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}
export default function ShareButton() {
  const [open, setOpen] = useState(false);
  const [copyState, setCopyState] = useState<"idle" | "ok" | "fail">("idle");

  // 水合后在客户端读取地址栏，自动适配任意部署域名
  const [url, setUrl] = useState("");
  useEffect(() => {
    setUrl(window.location.href);
  }, []);

  async function copyLink() {
    const ok = await copyText(url);
    setCopyState(ok ? "ok" : "fail");
    setTimeout(() => setCopyState("idle"), 2000);
  }

  return (
    <div className="relative inline-block">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={`inline-flex items-center gap-2 rounded-full border px-5 py-2 text-sm font-medium transition ${
          open
            ? "border-blue-200 bg-blue-50 text-blue-600"
            : "border-slate-200 bg-white text-slate-600 hover:border-blue-300 hover:text-blue-600 hover:shadow-sm"
        }`}
      >
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <circle cx="18" cy="5" r="3" />
          <circle cx="6" cy="12" r="3" />
          <circle cx="18" cy="19" r="3" />
          <line x1="8.59" y1="13.51" x2="15.42" y2="17.49" />
          <line x1="8.59" y1="10.49" x2="15.42" y2="6.51" />
        </svg>
        <span>分享</span>
      </button>

      {open && (
        <>
          {/* 透明遮罩：点击面板外区域关闭 */}
          <div
            className="fixed inset-0 z-10"
            onClick={() => setOpen(false)}
            aria-hidden="true"
          />
          <div className="absolute left-1/2 bottom-full z-20 mb-3 w-56 -translate-x-1/2 rounded-xl border border-slate-200 bg-white p-4 text-center shadow-lg">
            <QRCodeSVG value={url} size={160} marginSize={1} className="mx-auto" />
            <p className="mt-3 text-xs leading-relaxed text-slate-500">
              微信扫一扫，在微信中打开后可转发或分享到朋友圈
            </p>
            <button
              type="button"
              onClick={copyLink}
              className="mt-3 w-full rounded-lg border border-slate-200 px-3 py-1.5 text-xs text-slate-600 transition hover:border-blue-300 hover:text-blue-600"
            >
              {copyState === "ok" ? "已复制 ✓" : copyState === "fail" ? "复制失败，请长按链接复制" : "复制链接"}
            </button>
          </div>
        </>
      )}
    </div>
  );
}
