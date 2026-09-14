import { useEffect, useState } from 'react';
import { Card, Col, Row, Statistic, Table, Tag } from 'antd';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  FileTextOutlined,
  RocketOutlined,
} from '@ant-design/icons';
import { Link, useNavigate } from 'react-router-dom';
import { observer } from 'mobx-react-lite';
import dayjs from 'dayjs';
import { taskStore, articleStore } from '../stores';
import { ARTICLE_STATUS_TEXT, TASK_STATUS_TEXT } from '../types';
import type { GenTask } from '../types';

const TASK_COLOR: Record<string, string> = {
  pending: 'gold',
  running: 'processing',
  success: 'success',
  failed: 'error',
};

export const Dashboard = observer(function Dashboard() {
  const navigate = useNavigate();
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    void (async () => {
      try {
        // 拉取较大量用于本地统计（后端暂无聚合统计接口）
        await Promise.all([
          taskStore.fetchTasks(1, 100),
          articleStore.fetchArticles('', 1, 100),
        ]);
      } catch {
        // 首屏统计失败不阻塞页面
      } finally {
        setLoaded(true);
      }
    })();
  }, []);

  const tasks = taskStore.items;
  const articles = articleStore.items;
  const recentTasks = tasks.slice(0, 5);

  const stats = {
    taskTotal: taskStore.total || tasks.length,
    taskSuccess: tasks.filter((t) => t.status === 'success').length,
    taskFailed: tasks.filter((t) => t.status === 'failed').length,
    articleDraft: articles.filter((a) => a.status === 'draft').length,
    articlePublished: articles.filter((a) => a.status === 'published').length,
  };

  return (
    <div>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="任务总数"
              value={stats.taskTotal}
              prefix={<RocketOutlined />}
              loading={!loaded}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="任务成功"
              value={stats.taskSuccess}
              valueStyle={{ color: '#3f8600' }}
              prefix={<CheckCircleOutlined />}
              loading={!loaded}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="任务失败"
              value={stats.taskFailed}
              valueStyle={{ color: '#cf1322' }}
              prefix={<CloseCircleOutlined />}
              loading={!loaded}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="文章（待审核 / 已发布）"
              value={stats.articleDraft}
              suffix={` / ${stats.articlePublished}`}
              prefix={<FileTextOutlined />}
              loading={!loaded}
            />
          </Card>
        </Col>
      </Row>

      <Card
        title="最近任务"
        style={{ marginTop: 16 }}
        extra={<Link to="/tasks">查看全部</Link>}
      >
        <Table<GenTask>
          rowKey="id"
          size="small"
          loading={!loaded}
          dataSource={recentTasks}
          pagination={false}
          onRow={(record) => ({ onClick: () => navigate('/tasks'), style: { cursor: 'pointer' } })}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 64 },
            {
              title: 'Git 仓库',
              dataIndex: 'git_url',
              ellipsis: true,
            },
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
            { title: '进度', dataIndex: 'progress', width: 80, render: (v: number) => `${v}%` },
            {
              title: '创建时间',
              dataIndex: 'created_at',
              width: 180,
              render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '-'),
            },
          ]}
        />
        {articles.length > 0 && (
          <div style={{ marginTop: 12, color: '#999', fontSize: 12 }}>
            文章状态说明：{Object.entries(ARTICLE_STATUS_TEXT)
              .map(([k, v]) => `${k}=${v}`)
              .join('、')}
            ；下线后的文章回到 draft（待审核）状态。
          </div>
        )}
      </Card>
    </div>
  );
});
