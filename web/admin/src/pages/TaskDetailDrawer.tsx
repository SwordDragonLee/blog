import { useEffect, useRef } from 'react';
import { Alert, Button, Descriptions, Drawer, Progress, Space, Steps, Tag } from 'antd';
import { RedoOutlined } from '@ant-design/icons';
import { observer } from 'mobx-react-lite';
import dayjs from 'dayjs';
import { taskStore } from '../stores';
import { TASK_STATUS_TEXT } from '../types';

const TASK_COLOR: Record<string, string> = {
  pending: 'gold',
  running: 'processing',
  success: 'success',
  failed: 'error',
};

// 与后端 pipeline 的 prog.step 步骤名对齐（task/pipeline.go）：
// 开始/克隆仓库/仓库分析/LLM 分析/撰写文章/完成。
// 渲染配图在撰写文章循环内完成，不单列；审核属于文章状态机，不是任务阶段。
const STEPS = ['克隆仓库', '仓库分析', 'LLM 分析', '撰写文章', '完成'];

const STEP_KEYWORDS: RegExp[] = [
  /clone|克隆/i,
  /analy|scan|detect|仓库分析|采样/i,
  /llm|outline|选题|大纲/i,
  /writ|article|写作|撰写/i,
  /done|save|seeds|入库|完成/i,
];

function currentStepIndex(step: string, status: string): number {
  if (status === 'success') return STEPS.length - 1;
  if (!step) return 0;
  for (let i = 0; i < STEP_KEYWORDS.length; i++) {
    if (STEP_KEYWORDS[i].test(step)) return i;
  }
  return 0;
}

export const TaskDetailDrawer = observer(function TaskDetailDrawer({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const logRef = useRef<HTMLDivElement>(null);
  const task = taskStore.current;

  // 日志自动滚动到底部
  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [taskStore.logs.length]);

  const onRetry = async () => {
    if (!task) return;
    try {
      await taskStore.retry(task.id);
    } catch {
      // 重试失败信息在列表提示，这里静默
    }
  };

  return (
    <Drawer title={task ? `任务 #${task.id}` : '任务详情'} width={640} open={open} onClose={onClose}>
      {!task ? null : (
        <>
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="状态">
              <Space>
                <Tag color={TASK_COLOR[task.status] || 'default'}>
                  {TASK_STATUS_TEXT[task.status] || task.status}
                </Tag>
                {task.status === 'running' && <span>（2 秒轮询中）</span>}
              </Space>
            </Descriptions.Item>
            <Descriptions.Item label="Git 仓库">
              <span className="mono" style={{ fontSize: 12 }}>{task.git_url}</span>
            </Descriptions.Item>
            <Descriptions.Item label="进度">
              <Progress
                percent={task.progress}
                size="small"
                status={task.status === 'failed' ? 'exception' : task.status === 'success' ? 'success' : 'active'}
                style={{ maxWidth: 360 }}
              />
            </Descriptions.Item>
            <Descriptions.Item label="开始时间">
              {task.created_at ? dayjs(task.created_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
            </Descriptions.Item>
          </Descriptions>

          <Steps
            size="small"
            style={{ margin: '20px 0' }}
            direction="vertical"
            current={currentStepIndex(task.step, task.status)}
            status={task.status === 'failed' ? 'error' : task.status === 'success' ? 'finish' : 'process'}
            items={STEPS.map((label, i) => ({
              title: label,
              description:
                i === currentStepIndex(task.step, task.status) && task.message
                  ? task.message
                  : undefined,
            }))}
          />

          {task.status === 'failed' && (
            <Alert
              type="error"
              showIcon
              message="任务失败"
              description={task.error || '未知错误'}
              style={{ marginBottom: 16 }}
              action={
                <Button size="small" danger icon={<RedoOutlined />} onClick={() => void onRetry()}>
                  重试
                </Button>
              }
            />
          )}

          <div style={{ fontWeight: 500, marginBottom: 8 }}>执行日志（实时）</div>
          <div className="log-area mono" ref={logRef}>
            {taskStore.logs.length === 0 ? (
              <span style={{ color: '#64748b' }}>暂无日志</span>
            ) : (
              taskStore.logs.join('\n')
            )}
          </div>
        </>
      )}
    </Drawer>
  );
});
