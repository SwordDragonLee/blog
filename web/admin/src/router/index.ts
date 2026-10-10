/**
 * 路由模块出口：配置表在 ./config 维护，本文件只放从配置派生的
 * 菜单与高亮逻辑。业务代码统一从 '../router' 导入。
 */

export { APP_ROUTES, PUBLIC_ROUTES } from './config';

import { APP_ROUTES } from './config';

/** 侧边菜单项：有 title 的路由按配置顺序生成 */
export const MENU_ITEMS = APP_ROUTES.filter((r) => r.title).map((r) => ({
  key: r.path,
  icon: r.icon,
  label: r.title,
}));

/** 当前路径对应的菜单高亮 key：取「有 title 且 path 是前缀」的最长匹配，
 *  让 /tasks/3/logs 高亮「任务」、/articles/1/edit 高亮「文章管理」。 */
export function matchMenuKey(pathname: string): string {
  const hits = APP_ROUTES.filter(
    (r) => r.title && (pathname === r.path || pathname.startsWith(`${r.path}/`)),
  );
  return hits.sort((a, b) => b.path.length - a.path.length)[0]?.path ?? '/';
}
