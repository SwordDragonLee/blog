import { useState } from 'react';
import { Form, Input, Modal, message } from 'antd';
import { taskStore } from '../stores';

// git_url 必须以 http(s):// 或 git@ 开头
const GIT_URL_PATTERN = /^(https?:\/\/.+|git@.+)/;

export function CreateTaskModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: (taskId: number) => void;
}) {
  const [form] = Form.useForm<{ git_url: string }>();
  const [creating, setCreating] = useState(false);

  const onOk = async () => {
    try {
      const values = await form.validateFields();
      setCreating(true);
      const task = await taskStore.createTask(values.git_url.trim());
      message.success('任务已创建');
      form.resetFields();
      if (task?.id) onCreated(task.id);
    } catch (e) {
      // 表单校验失败时 validateFields 抛出的错误无 message，跳过提示
      const msg = (e as Error).message;
      if (msg) message.error(msg);
    } finally {
      setCreating(false);
    }
  };

  return (
    <Modal
      title="新建生成任务"
      open={open}
      onOk={onOk}
      confirmLoading={creating}
      onCancel={onClose}
      okText="创建"
      cancelText="取消"
      destroyOnClose
    >
      <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
        <Form.Item
          name="git_url"
          label="Git 仓库地址（公开仓库）"
          rules={[
            { required: true, message: '请输入 Git 仓库地址' },
            {
              validator: (_rule, value: string) =>
                !value || GIT_URL_PATTERN.test(value.trim())
                  ? Promise.resolve()
                  : Promise.reject(new Error('必须以 http(s):// 或 git@ 开头')),
            },
          ]}
        >
          <Input placeholder="https://github.com/user/repo.git" allowClear />
        </Form.Item>
      </Form>
    </Modal>
  );
}
