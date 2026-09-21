import type { NextConfig } from "next";

const apiBase = process.env.API_BASE ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  // 生产镜像用 standalone 产物（.next/standalone 最小运行时），本地 dev/ build 不受影响
  output: "standalone",
  async rewrites() {
    // 让客户端 / <img> 可以直接用相对路径 /api/v1/... 访问 Go 后端，
    // 避免跨域；服务端（RSC）则通过 lib/api.ts 用绝对地址直连。
    return [
      {
        source: "/api/:path*",
        destination: `${apiBase}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
