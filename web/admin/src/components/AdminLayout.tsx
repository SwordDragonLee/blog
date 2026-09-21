import { useEffect, useState } from 'react';
import { Layout, Menu, Modal, Input, Popconfirm, Typography, message } from 'antd';
import {
  DatabaseOutlined,
  DashboardOutlined,
  FileTextOutlined,
  RocketOutlined,
  LogoutOutlined,
  MailOutlined,
} from '@ant-design/icons';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { observer } from 'mobx-react-lite';
import { authStore } from '../stores';
import { get, put } from '../api/http';
import type { AdminUser } from '../types';

const { Sider, Header, Content } = Layout;

const MENU_ITEMS = [
  { key: '/', icon: <DashboardOutlined />, label: '工作台' },
  { key: '/tasks', icon: <RocketOutlined />, label: '任务' },
  { key: '/articles', icon: <FileTextOutlined />, label: '文章管理' },
  { key: '/rag-index', icon: <DatabaseOutlined />, label: 'RAG 索引' },
];

export const AdminLayout = observer(function AdminLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  // 邮箱设置弹窗：加载当前邮箱，保存后同步 authStore
  const [mailOpen, setMailOpen] = useState(false);
  const [email, setEmail] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!mailOpen) return;
    void get<AdminUser>('/auth/profile').then((u) => setEmail(u.email || ''));
  }, [mailOpen]);

  const saveEmail = async () => {
    setSaving(true);
    try {
      await put<AdminUser>('/auth/profile', { email: email.trim() });
      message.success('邮箱已保存');
      setMailOpen(false);
    } catch (e) {
      message.error((e as Error).message || '保存失败');
    } finally {
      setSaving(false);
    }
  };

  const selectedKey = location.pathname.startsWith('/tasks')
    ? '/tasks'
    : location.pathname.startsWith('/articles')
      ? '/articles'
      : location.pathname.startsWith('/rag-index')
        ? '/rag-index'
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
          <a
            style={{ marginRight: 12 }}
            onClick={() => {
              setEmail('');
              setMailOpen(true);
            }}
          >
            <MailOutlined /> 邮箱设置
          </a>
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
      <Modal
        title="邮箱设置"
        open={mailOpen}
        onOk={saveEmail}
        onCancel={() => setMailOpen(false)}
        confirmLoading={saving}
        okText="保存"
        cancelText="取消"
        destroyOnHidden
      >
        <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
          用于接收文章发布成功的通知邮件。
        </Typography.Paragraph>
        <Input
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="name@example.com"
          maxLength={128}
        />
      </Modal>
    </Layout>
  );
});
