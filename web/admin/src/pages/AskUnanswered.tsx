import { useCallback, useEffect, useState } from 'react';
import { Button, DatePicker, Form, Input, Table, Typography, message } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import type { Dayjs } from 'dayjs';
import { get } from '../api/http';
import type { AskUnanswered, PageData } from '../types';

interface QueryForm {
  keyword?: string;
  range?: [Dayjs, Dayjs] | null;
}

/** 无命中问题列表：同一归一化键聚合，按提问次数降序；选题回流的原料清单 */
export function AskUnansweredPage() {
  const [items, setItems] = useState<AskUnanswered[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(false);
  // 已提交的查询条件：点【搜索】才生效，表单里改动不影响当前列表
  const [keyword, setKeyword] = useState('');
  const [range, setRange] = useState<[Dayjs, Dayjs] | null>(null);
  const [form] = Form.useForm<QueryForm>();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await get<PageData<AskUnanswered>>('/rag/unanswered', {
        page,
        page_size: pageSize,
        ...(keyword ? { keyword } : {}),
        ...(range ? { start: range[0].format('YYYY-MM-DD'), end: range[1].format('YYYY-MM-DD') } : {}),
      });
      setItems(data.items ?? []);
      setTotal(data.total ?? 0);
    } catch (e) {
      message.error((e as Error).message || '加载失败');
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, keyword, range]);

  useEffect(() => {
    void load();
  }, [load]);

  // 搜索：提交表单条件并回到第一页
  const onSearch = (values: QueryForm) => {
    setKeyword((values.keyword ?? '').trim());
    setRange(values.range ?? null);
    setPage(1);
  };

  // 重置：清空表单与已生效条件，回到第一页
  const onReset = () => {
    form.resetFields();
    setKeyword('');
    setRange(null);
    setPage(1);
  };

  const columns = [
    {
      title: '提问次数',
      dataIndex: 'hits',
      width: 100,
      render: (n: number) => (n > 1 ? <b style={{ color: '#c7a008' }}>{n} 次</b> : '1 次'),
    },
    { title: '问题', dataIndex: 'question', ellipsis: true },
    {
      title: '首次提问',
      dataIndex: 'first_seen_at',
      width: 170,
      render: (v: string) => (v ? new Date(v).toLocaleString() : '-'),
    },
    {
      title: '最近提问',
      dataIndex: 'last_seen_at',
      width: 170,
      render: (v: string) => (v ? new Date(v).toLocaleString() : '-'),
    },
  ];

  return (
    <div>
      <Typography.Title level={4} style={{ marginTop: 0 }}>
        无命中问题
      </Typography.Title>
      <Form form={form} layout="inline" onFinish={onSearch} style={{ marginBottom: 16 }}>
        <Form.Item name="keyword" label="问题关键词">
          <Input allowClear placeholder="搜索问题或归一化键" style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="range" label="提问时间">
          <DatePicker.RangePicker allowClear placeholder={['提问不早于', '提问不晚于']} />
        </Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit" icon={<SearchOutlined />} loading={loading}>
            搜索
          </Button>
          <Button htmlType="button" onClick={onReset} style={{ marginLeft: 8 }}>
            重置
          </Button>
        </Form.Item>
      </Form>
      <Table
        rowKey="id"
        columns={columns}
        dataSource={items}
        loading={loading}
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: false,
          onChange: (p, ps) => {
            setPage(p);
            setPageSize(ps);
          },
        }}
      />
    </div>
  );
}
