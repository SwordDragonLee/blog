import { Layout, Menu, Popconfirm, Typography } from 'antd';
import {
  DashboardOutlined,
  FileTextOutlined,
  RocketOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { observer } from 'mobx-react-lite';
import { authStore } from '../stores';

const { Sider, Header, Content } = Layout;

const MENU_ITEMS = [
  { key: '/', icon: <DashboardOutlined />, label: '工作台' },
  { key: '/tasks', icon: <RocketOutlined />, label: '任务' },
  { key: '/articles', icon: <FileTextOutlined />, label: '文章管理' },
];

export const AdminLayout = observer(function AdminLayout() {
  const navigate = useNavigate();
  const location = useLocation();

  const selectedKey =
    location.pathname.startsWith('/tasks')
      ? '/tasks'
      : location.pathname.startsWith('/articles')
        ? '/articles'
        : '/';

  const onLogout = () => {
    authStore.logout();
    navigate('/login', { replace: true });
  };

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider theme="dark" width={208}>
        <div style={{ color: '#fff', padding: '18px 16px', fontWeight: 600, fontSize: 16 }}>
          AI 博客管理平台
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          items={MENU_ITEMS}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            background: '#fff',
            padding: '0 24px',
            display: 'flex',
            justifyContent: 'flex-end',
            alignItems: 'center',
            borderBottom: '1px solid #f0f0f0',
          }}
        >
          <Typography.Text type="secondary" style={{ marginRight: 12 }}>
            {authStore.username}
          </Typography.Text>
          <Popconfirm title="确定退出登录？" onConfirm={onLogout}>
            <a>
              <LogoutOutlined /> 退出
            </a>
          </Popconfirm>
        </Header>
        <Content style={{ margin: 16 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
});
