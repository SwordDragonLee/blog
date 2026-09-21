import { useEffect } from 'react';
import { Card, Col, Descriptions, Form, Input, Row, Select, Tag } from 'antd';
import type { FormInstance } from 'antd';
import dayjs from 'dayjs';
import { ARTICLE_STATUS_TEXT } from '../types';
import type { ArticleDetail } from '../types';

export interface ArticleMetaValues {
  title: string;
  summary: string;
  tags: string[];
}

const STATUS_COLOR: Record<string, string> = {
  draft: 'gold',
  published: 'success',
  offline: 'default',
};

/** 文章顶部信息卡：标题 / 摘要 / 标签编辑 + 元信息展示 */
export function ArticleMetaForm({
  form,
  current,
  loading,
}: {
  form: FormInstance<ArticleMetaValues>;
  current: ArticleDetail | null;
  loading: boolean;
}) {
  // 详情变化（加载/保存/发版/下线后）同步到表单
  useEffect(() => {
    if (current) {
      form.setFieldsValue({
        title: current.title,
        summary: current.summary,
        tags: current.tags || [],
      });
    }
  }, [current, form]);

  return (
    <Card loading={loading} style={{ marginBottom: 16 }}>
      <Form form={form} layout="vertical">
        <Row gutter={16}>
          <Col xs={24} md={14}>
            <Form.Item
              name="title"
              label="标题"
              rules={[{ required: true, message: '标题不能为空' }]}
            >
              <Input placeholder="文章标题" />
            </Form.Item>
          </Col>
          <Col xs={24} md={10}>
            <Form.Item name="tags" label="标签">
              <Select mode="tags" tokenSeparators={[',']} placeholder="输入后回车添加标签" open={false} />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="summary" label="摘要">
          <Input.TextArea autoSize={{ minRows: 1, maxRows: 4 }} placeholder="文章摘要" />
        </Form.Item>
      </Form>
      {current && (
        <Descriptions size="small" column={4}>
          <Descriptions.Item label="Slug">
            <span className="mono">{current.slug}</span>
          </Descriptions.Item>
          <Descriptions.Item label="字数">{current.word_count}</Descriptions.Item>
          <Descriptions.Item label="状态">
            <Tag color={STATUS_COLOR[current.status] || 'default'}>
              {ARTICLE_STATUS_TEXT[current.status] || current.status}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="发布时间">
            {current.published_at ? dayjs(current.published_at).format('YYYY-MM-DD HH:mm') : '-'}
          </Descriptions.Item>
        </Descriptions>
      )}
    </Card>
  );
}
