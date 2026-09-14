import { Button, Card, Form, Input, Typography, message } from 'antd';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { observer } from 'mobx-react-lite';
import { authStore } from '../stores';

interface LoginForm {
  username: string;
  password: string;
}

export const Login = observer(function Login() {
  const navigate = useNavigate();

  const onFinish = async (values: LoginForm) => {
    try {
      await authStore.login(values.username, values.password);
      navigate('/', { replace: true });
    } catch (err) {
      // 错误信息由 http 拦截器统一产出
      message.error((err as Error).message || '登录失败');
    }
  };

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: '#f0f2f5',
      }}
    >
      <Card style={{ width: 380 }}>
        <Typography.Title level={3} style={{ textAlign: 'center', marginBottom: 24 }}>
          AI 博客管理平台
        </Typography.Title>
        <Form<LoginForm> onFinish={onFinish} size="large" autoComplete="off">
          <Form.Item
            name="username"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input prefix={<UserOutlined />} placeholder="用户名" />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="密码" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 8 }}>
            <Button type="primary" htmlType="submit" block loading={authStore.loggingIn}>
              登录
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
});
