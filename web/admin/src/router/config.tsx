import {
  DatabaseOutlined,
  DashboardOutlined,
  FileTextOutlined,
  QuestionCircleOutlined,
  RocketOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Login } from '../pages/Login';
import { Dashboard } from '../pages/Dashboard';
import { Tasks } from '../pages/Tasks';
import { TaskLogs } from '../pages/TaskLogs';
import { Articles } from '../pages/Articles';
import { ArticleEdit } from '../pages/ArticleEdit';
import { RagIndex } from '../pages/RagIndex';
import { AskUnansweredPage } from '../pages/AskUnanswered';
import { AccountSettings } from '../pages/AccountSettings';
import type { AppRoute } from '../types';

/**
 * 路由配置表（纯配置，派生逻辑见 ./index）：加页面 = 这里加一条。
 * - PUBLIC_ROUTES：无登录布局的页面（登录页）；
 * - APP_ROUTES：AdminLayout 下的业务页面，带 title 的自动进侧边菜单，
 *   详情/编辑类页面（title 省略）只注册路由不进菜单。
 */

export const PUBLIC_ROUTES: AppRoute[] = [{ path: '/login', element: <Login /> }];

export const APP_ROUTES: AppRoute[] = [
  { path: '/', title: '工作台', icon: <DashboardOutlined />, element: <Dashboard /> },
  { path: '/tasks', title: '任务', icon: <RocketOutlined />, element: <Tasks /> },
  { path: '/tasks/:id/logs', element: <TaskLogs /> },
  { path: '/articles', title: '文章管理', icon: <FileTextOutlined />, element: <Articles /> },
  { path: '/articles/:id/edit', element: <ArticleEdit /> },
  { path: '/rag-index', title: 'RAG 索引', icon: <DatabaseOutlined />, element: <RagIndex /> },
  {
    path: '/ask-unanswered',
    title: '无命中问题',
    icon: <QuestionCircleOutlined />,
    element: <AskUnansweredPage />,
  },
  {
    path: '/account',
    title: '账户设置',
    icon: <UserOutlined />,
    element: <AccountSettings />,
  },
];
