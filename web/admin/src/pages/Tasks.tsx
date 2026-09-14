import { useEffect, useState } from 'react';
import { Button, Progress, Table, Tag, Tooltip, message } from 'antd';
import { PlusOutlined, RedoOutlined, EyeOutlined } from '@ant-design/icons';
import { observer } from 'mobx-react-lite';
import dayjs from 'dayjs';
import { taskStore } from '../stores';
import { TASK_STATUS_TEXT } from '../types';
import type { GenTask } from '../types';
import { TaskDetailDrawer } from './TaskDetailDrawer';
import { CreateTaskModal } from './CreateTaskModal';

const TASK_COLOR: Record<string, string> = {
  pending: 'gold',
  running: 'processing',
  success: 'success',
  failed: 'error',
};

export const Tasks = observer(function Tasks() {
  const [createOpen, setCreateOpen] = useState(false);
  const [detailId, setDetailId] = useState<number | null>(null);

  useEffect(() => {
    void taskStore.fetchTasks(1, taskStore.pageSize).catch((e: Error) => message.error(e.message));
    return () => taskStore.stopPolling();
  }, []);

  const openDetail = async (id: number) => {
    setDetailId(id);
    try {
      const task = await taskStore.fetchTask(id);
      if (task.status === 'pending' || task.status === 'running') {
        taskStore.startPolling(id);
      }
    } catch (e) {
      message.error((e as Error).message);
    }
  };

  const onRetry = async (id: number) => {
    try {
      await taskStore.retry(id);
      message.success('已重新投递任务');
    } catch (e) {
      message.error((e as Error).message || '重试失败');
    }
  };

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
        <h3 style={{ margin: 0 }}>生成任务</h3>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
          新建任务
        </Button>
      </div>

      <Table<GenTask>
        rowKey="id"
        loading={taskStore.loading}
        dataSource={taskStore.items}
        pagination={{
          current: taskStore.page,
          pageSize: taskStore.pageSize,
          total: taskStore.total,
          showSizeChanger: true,
          onChange: (page, pageSize) =>
            taskStore
              .fetchTasks(page, pageSize)
              .catch((e: Error) => message.error(e.message)),
        }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 64 },
          { title: 'Git 仓库', dataIndex: 'git_url', ellipsis: true },
          {
            title: '状态',
            dataIndex: 'status',
            width: 96,
            render: (status: string) => (
              <Tag color={TASK_COLOR[status] || 'default'}>
                {TASK_STATUS_TEXT[status] || status}
              </Tag>
            ),
          },
          {
            title: '进度',
            dataIndex: 'progress',
            width: 140,
            render: (v: number, record) => (
              <Progress
                percent={v}
                size="small"
                status={record.status === 'failed' ? 'exception' : undefined}
              />
            ),
          },
          { title: '当前步骤', dataIndex: 'step', width: 120, ellipsis: true },
          {
            title: '信息 / 错误',
            dataIndex: 'message',
            ellipsis: true,
            render: (text: string, record) => {
              const tip = record.error || text;
              if (!tip) return '-';
              return (
                <Tooltip title={tip}>
                  <span style={{ color: record.error ? '#cf1322' : undefined }}>
                    {record.error || text}
                  </span>
                </Tooltip>
              );
            },
          },
          {
            title: '创建时间',
            dataIndex: 'created_at',
            width: 170,
            render: (v: string) => (v ? dayjs(v).format('MM-DD HH:mm:ss') : '-'),
          },
          {
            title: '操作',
            key: 'action',
            width: 150,
            render: (_, record) => (
              <>
                <Button
                  type="link"
                  size="small"
                  icon={<EyeOutlined />}
                  onClick={() => void openDetail(record.id)}
                >
                  查看
                </Button>
                {record.status === 'failed' && (
                  <Button
                    type="link"
                    size="small"
                    icon={<RedoOutlined />}
                    onClick={() => void onRetry(record.id)}
                  >
                    重试
                  </Button>
                )}
              </>
            ),
          },
        ]}
      />

      <CreateTaskModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(taskId) => void openDetail(taskId)}
      />

      <TaskDetailDrawer
        open={detailId !== null}
        onClose={() => {
          setDetailId(null);
          taskStore.resetCurrent();
        }}
      />
    </div>
  );
});
