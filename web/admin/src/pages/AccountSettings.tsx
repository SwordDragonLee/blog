import { useState } from 'react';
import { Button, Card, Form, Input, Typography, message } from 'antd';
import { useNavigate } from 'react-router-dom';
import { observer } from 'mobx-react-lite';
import { authStore } from '../stores';
import { put } from '../api/http';
import type { AdminUser, LoginResp } from '../types';

interface ProfileForm {
  username: string;
  email?: string;
}

interface PasswordForm {
  old_password: string;
  new_password: string;
  confirm: string;
}

/** 账户设置：用户信息（用户名 + 邮箱）与修改密码两个独立表单 */
export const AccountSettings = observer(function AccountSettings() {
  const navigate = useNavigate();
  const [savingProfile, setSavingProfile] = useState(false);
  const [savingPwd, setSavingPwd] = useState(false);
  const [pwdForm] = Form.useForm<PasswordForm>();

  // 保存用户信息：只对有变更的字段发请求；改用户名会重签 JWT，无感换发会话
  const saveProfile = async (values: ProfileForm) => {
    const username = values.username.trim();
    const email = (values.email ?? '').trim();
    // 提交前快照，作为 diff 基准（改用户名的响应会刷新 store，不影响后续判断）
    const prevUsername = authStore.username;
    const prevEmail = authStore.user?.email ?? '';
    if (username === prevUsername && email === prevEmail) {
      message.info('资料未变更');
      return;
    }
    setSavingProfile(true);
    try {
      if (username !== prevUsername) {
        const data = await put<LoginResp>('/auth/username', { username });
        authStore.setSession(data.token, data.user);
      }
      if (email !== prevEmail) {
        const user = await put<AdminUser>('/auth/profile', { email });
        authStore.updateUser(user);
      }
      message.success('用户信息已保存');
    } catch (e) {
      message.error((e as Error).message || '保存失败');
    } finally {
      setSavingProfile(false);
    }
  };

  // 改密码：成功后清除本地登录态，强制重新登录
  const savePassword = async (values: PasswordForm) => {
    setSavingPwd(true);
    try {
      await put('/auth/password', {
        old_password: values.old_password,
        new_password: values.new_password,
      });
      message.success('密码已修改，请重新登录');
      authStore.logout();
      navigate('/login', { replace: true });
    } catch (e) {
      message.error((e as Error).message || '修改失败');
    } finally {
      setSavingPwd(false);
    }
  };

  return (
    <div style={{ maxWidth: 560 }}>
      <Typography.Title level={4} style={{ marginTop: 0 }}>
        账户设置
      </Typography.Title>

      <Card title="用户信息" style={{ marginBottom: 16 }}>
        <Form
          layout="vertical"
          initialValues={{ username: authStore.username, email: authStore.user?.email }}
          onFinish={saveProfile}
        >
          <Form.Item<ProfileForm>
            name="username"
            label="用户名"
            rules={[
              { required: true, whitespace: true, message: '请输入用户名' },
              { max: 64, message: '用户名不能超过 64 个字符' },
            ]}
          >
            <Input placeholder="登录用户名" maxLength={64} />
          </Form.Item>
          <Form.Item<ProfileForm>
            name="email"
            label="通知邮箱"
            extra="用于接收文章发布成功的通知邮件，留空表示解除绑定。"
            rules={[{ type: 'email', message: '邮箱格式不正确' }]}
          >
            <Input placeholder="name@example.com" maxLength={128} allowClear />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={savingProfile}>
            保存
          </Button>
        </Form>
      </Card>

      <Card title="修改密码">
        <Form form={pwdForm} layout="vertical" onFinish={savePassword}>
          <Form.Item<PasswordForm>
            name="old_password"
            label="旧密码"
            rules={[{ required: true, message: '请输入旧密码' }]}
          >
            <Input.Password placeholder="当前密码" maxLength={64} />
          </Form.Item>
          <Form.Item<PasswordForm>
            name="new_password"
            label="新密码"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 6, message: '新密码至少 6 位' },
              { max: 64, message: '新密码最多 64 位' },
            ]}
          >
            <Input.Password placeholder="6~64 位" maxLength={64} />
          </Form.Item>
          <Form.Item<PasswordForm>
            name="confirm"
            label="确认新密码"
            dependencies={['new_password']}
            rules={[
              { required: true, message: '请再次输入新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('new_password') === value) {
                    return Promise.resolve();
                  }
                  return Promise.reject(new Error('两次输入的密码不一致'));
                },
              }),
            ]}
          >
            <Input.Password placeholder="再次输入新密码" maxLength={64} />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={savingPwd}>
            保存
          </Button>
          <Typography.Text type="secondary" style={{ marginLeft: 12 }}>
            修改成功后将退出登录，需用新密码重新登录。
          </Typography.Text>
        </Form>
      </Card>
    </div>
  );
});
