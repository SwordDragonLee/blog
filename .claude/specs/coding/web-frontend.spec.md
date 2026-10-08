# 前端编码规范（web/admin + web/blog）

> 硬约束。存量代码不符合时按「顺手修复、单独提交」处理，不做无关重构。

## 通用

- React 函数组件 + Hooks；TypeScript；**禁止**新增 class 组件、`any` 滥用
- 用户可见文案中文；标识符英文
- 提交前必须过对应目录 `npm run build`（admin = tsc --noEmit + vite build；blog = next build）

## admin（React 18 + Antd 5 + MobX）

- HTTP 请求**必须**走 `api/http.ts` 的 axios 实例（统一带 JWT、401 跳登录、错误提示），**禁止**组件里裸建 fetch / 新 axios 实例
- 与后端对齐的共享类型放 `types.ts`，**禁止**在页面里手写重复 interface
- 全局状态走 MobX store（`stores/`，observer 订阅）；仅组件内部状态用 useState，不把所有状态都塞 store
- 页面放 `pages/`，路由在 `App.tsx` / 路由配置中注册

```tsx
// ❌ 禁止：组件内裸写请求
const res = await axios.get('http://localhost:8080/api/v1/articles')

// ✅ 正确：走统一实例 + 共享类型
import http from '../api/http'
import type { Article } from '../types'
const { data } = await http.get<{ items: Article[] }>('/api/v1/articles')
```

## blog（Next.js App Router + Tailwind 4）

- 服务端取数统一走 `lib/api`（后端地址读 `API_BASE`），**禁止**组件里硬编码 `localhost:8080` 或绝对域名
- 页面 = `app/` 下的 `page.tsx` + `generateMetadata`（约定式导出，框架调用）；文章页保持 SSR + ISR（`export const revalidate`），新增列表页默认同样处理
- Markdown 渲染统一 react-markdown + remark-gfm + rehype-highlight，不引入第二套渲染器
- 样式用 Tailwind 工具类；复杂视觉（渐变卡片等）可抽组件常量，**禁止**内联 style 拼长字符串
- 客户端组件显式 `"use client"`；能服务端渲染的不下沉到客户端
