import { useEffect, useState } from 'react';
import { Button, Popconfirm, Progress, Table, Tag, Tooltip, message } from 'antd';
import {
  DeleteOutlined,
  EyeOutlined,
  PlusOutlined,
  RedoOutlined,
  StopOutlined,
} from '@ant-design/icons';
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
  canceled: 'default',
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

  const onCancel = async (id: number) => {
    try {
      await taskStore.cancel(id);
      message.success('任务已取消');
    } catch (e) {
      message.error((e as Error).message || '取消失败');
    }
  };

  const onDelete = async (id: number) => {
    try {
      await taskStore.remove(id);
      message.success('任务已删除');
    } catch (e) {
      message.error((e as Error).message || '删除失败');
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
            width: 210,
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
                {(record.status === 'pending' || record.status === 'running') && (
                  <Popconfirm
                    title="确定取消该任务？"
                    description="运行中的任务会尽快中断，已生成的草稿文章会被保留。"
                    onConfirm={() => void onCancel(record.id)}
                  >
                    <Button type="link" size="small" icon={<StopOutlined />} danger>
                      取消
                    </Button>
                  </Popconfirm>
                )}
                {['success', 'failed', 'canceled'].includes(record.status) && (
                  <Popconfirm
                    title="删除该任务？"
                    description="将删除任务记录、分析记录、草稿文章与配图；有已发布文章时需先下线。"
                    onConfirm={() => void onDelete(record.id)}
                  >
                    <Button type="link" size="small" icon={<DeleteOutlined />} danger>
                      删除
                    </Button>
                  </Popconfirm>
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
