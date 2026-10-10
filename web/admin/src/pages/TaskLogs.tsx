import { useEffect, useRef } from 'react';
import { Button, Space, Spin, Tag, message } from 'antd';
import { ArrowLeftOutlined, ReloadOutlined } from '@ant-design/icons';
import { observer } from 'mobx-react-lite';
import { useNavigate, useParams } from 'react-router-dom';
import { taskStore } from '../stores';
import { TASK_STATUS_TEXT } from '../types';

const TASK_COLOR: Record<string, string> = {
  pending: 'gold',
  running: 'processing',
  success: 'success',
  failed: 'error',
  canceled: 'default',
};

// 任务日志独立页：整页高度展示执行日志，运行中的任务沿用 taskStore 的
// 2 秒轮询（与抽屉视图共享状态，加载/停止逻辑一致）。
export const TaskLogs = observer(function TaskLogs() {
  const { id } = useParams();
  const navigate = useNavigate();
  const logRef = useRef<HTMLDivElement>(null);
  const taskId = Number(id);

  useEffect(() => {
    if (!Number.isInteger(taskId) || taskId <= 0) return;
    taskStore
      .fetchTask(taskId)
      .then((task) => {
        if (task.status === 'pending' || task.status === 'running') {
          taskStore.startPolling(taskId);
        }
      })
      .catch((e: Error) => message.error(e.message));
    return () => taskStore.stopPolling();
  }, [taskId]);

  // 日志自动滚动到底部
  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [taskStore.logs.length]);

  const task = taskStore.current;
  const isCurrent = task?.id === taskId;

  const onRefresh = async () => {
    try {
      await taskStore.fetchTask(taskId);
    } catch (e) {
      message.error((e as Error).message);
    }
  };

  return (
    <div>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 12,
        }}
      >
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/tasks')}>
            返回
          </Button>
          <h3 style={{ margin: 0 }}>任务 #{taskId} 执行日志</h3>
          {isCurrent && (
            <Tag color={TASK_COLOR[task.status] || 'default'}>
              {TASK_STATUS_TEXT[task.status] || task.status}
            </Tag>
          )}
          {isCurrent && task.status === 'running' && (
            <span style={{ color: '#64748b' }}>2 秒自动刷新</span>
          )}
        </Space>
        <Button icon={<ReloadOutlined />} onClick={() => void onRefresh()}>
          刷新
        </Button>
      </div>
      {isCurrent && (
        <div className="mono" style={{ fontSize: 12, color: '#64748b', marginBottom: 8 }}>
          {task.git_url}
        </div>
      )}
      <div className="log-area log-area-full mono" ref={logRef}>
        {!isCurrent ? (
          <Spin size="small" />
        ) : taskStore.logs.length === 0 ? (
          <span style={{ color: '#64748b' }}>暂无日志</span>
        ) : (
          taskStore.logs.join('\n')
        )}
      </div>
    </div>
  );
});
