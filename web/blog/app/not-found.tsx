import Link from "next/link";

export default function NotFound() {
  return (
    <main className="mx-auto flex w-full max-w-3xl flex-1 flex-col items-center justify-center px-4 py-24 text-center">
      <p className="text-6xl font-black tracking-widest text-slate-200">404</p>
      <h1 className="mt-5 text-xl font-bold text-slate-800">
        文章不存在或已下线
      </h1>
      <p className="mt-2 text-sm text-slate-500">
        内容可能正在生成中，稍后再来看看。
      </p>
      <Link
        href="/"
        className="mt-8 rounded-lg bg-blue-600 px-5 py-2.5 text-sm font-medium text-white shadow-sm transition hover:bg-blue-700"
      >
        返回首页
      </Link>
    </main>
  );
}
