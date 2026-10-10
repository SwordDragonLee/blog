// 跨业务复用的通用类型
import type { ReactNode } from 'react';

// 统一分页响应 data
export interface PageData<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

/** 前端路由配置项（routes.tsx 的条目类型）：title 省略 = 不进侧边菜单 */
export interface AppRoute {
  path: string;
  element: ReactNode;
  title?: string;
  icon?: ReactNode;
}
