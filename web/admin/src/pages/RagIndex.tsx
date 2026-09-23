import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Card,
  Empty,
  Input,
  InputNumber,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { get } from '../api/http';
import type { RagIndexOverview, RagProbeHit } from '../types';

const { Search } = Input;

// 检索测试默认返回条数与后端 limit 上限对齐
const PROBE_LIMIT = 10;

// 检索测试命中表列定义
const probeColumns: ColumnsType<RagProbeHit> = [
  {
    title: '得分',
    dataIndex: 'score',
    width: 90,
    render: (score: number) => score.toFixed(4),
  },
  {
    title: '文章',
    dataIndex: 'title',
    width: 280,
    ellipsis: true,
  },
  {
    title: '片段摘要',
    dataIndex: 'snippet',
    ellipsis: true,
  },
  {
    title: '阈值判定',
    dataIndex: 'pass',
    width: 100,
    render: (pass: boolean) =>
      pass ? <Tag color="success">采纳</Tag> : <Tag>过滤</Tag>,
  },
];

export function RagIndex() {
  const [overview, setOverview] = useState<RagIndexOverview | null>(null);
  const [loadErr, setLoadErr] = useState('');
  const [probing, setProbing] = useState(false);
  const [question, setQuestion] = useState('');
  const [threshold, setThreshold] = useState(0.5);
  const [hits, setHits] = useState<RagProbeHit[] | null>(null);

  useEffect(() => {
    void get<RagIndexOverview>('/rag/index')
      .then(setOverview)
      .catch((e) => setLoadErr((e as Error).message));
  }, []);

  const onProbe = useCallback(
    async (q?: string) => {
      const text = (q ?? question).trim();
      if (!text) return;
      setProbing(true);
      try {
        const list = await get<RagProbeHit[]>('/rag/probe', {
          q: text,
          threshold,
          limit: PROBE_LIMIT,
        });
        setHits(list);
      } catch (e) {
        message.error((e as Error).message || '检索测试失败');
      } finally {
        setProbing(false);
      }
    },
    [question, threshold],
  );

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      {loadErr && <Alert type="error" showIcon message="索引总览加载失败" description={loadErr} />}

      <Card title="向量索引总览" size="small">
        {overview ? (
          <>
            <Table
              size="small"
              rowKey="slug"
              columns={[
                { title: '文章标题', dataIndex: 'title', ellipsis: true },
                { title: 'slug', dataIndex: 'slug', ellipsis: true, width: 320 },
                { title: '向量块数', dataIndex: 'chunks', width: 100 },
              ]}
              dataSource={overview.articles}
              pagination={{ pageSize: 20, hideOnSinglePage: true, showSizeChanger: false, showTotal: (n) => `共 ${n} 篇` }}
            />
            <Typography.Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
              共 {overview.articles.length} 篇文章、{overview.total_chunks} 个向量块。
              已发布文章未出现在列表中即漏索引；已下线文章仍出现即残留向量。
            </Typography.Text>
          </>
        ) : (
          !loadErr && <Empty description="加载中…" />
        )}
      </Card>

      <Card title="检索测试" size="small">
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Space wrap>
            <Search
              style={{ width: 480 }}
              placeholder="输入问题，观察命中的文章与得分分布"
              enterButton="检索"
              allowClear
              value={question}
              onChange={(e) => setQuestion(e.target.value)}
              onSearch={(v) => void onProbe(v)}
            />
            <Space.Compact>
              <Typography.Text type="secondary" style={{ padding: '5px 8px 0 0' }}>
                阈值
              </Typography.Text>
              <InputNumber
                min={0}
                max={1}
                step={0.05}
                value={threshold}
                onChange={(v) => setThreshold(v ?? 0.5)}
              />
            </Space.Compact>
          </Space>
          <Typography.Text type="secondary">
            得分 ≥ 阈值记为「采纳」（与问答接口 score_threshold 同规则）；改阈值只影响本页判定，不修改后端配置。
          </Typography.Text>
          <Table
            size="small"
            rowKey={(r) => `${r.slug}-${r.score}`}
            columns={probeColumns}
            dataSource={hits ?? []}
            loading={probing}
            pagination={false}
            locale={{ emptyText: '输入问题后点「检索」观察命中分布' }}
          />
        </Space>
      </Card>
    </Space>
  );
}
