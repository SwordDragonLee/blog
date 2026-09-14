import { useEffect } from 'react';
import { Button, Table, Tabs, Tag, Tooltip, message } from 'antd';
import { EditOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { observer } from 'mobx-react-lite';
import dayjs from 'dayjs';
import { articleStore } from '../stores';
import { ARTICLE_STATUS_TEXT } from '../types';
import type { Article, ArticleStatus } from '../types';

const STATUS_COLOR: Record<string, string> = {
  draft: 'gold',
  published: 'success',
  offline: 'default',
};

// 后端 status 枚举：draft/published/offline；下线后即回到 draft，故筛选用 全部/draft/published
const TAB_ITEMS = [
  { key: '', label: '全部' },
  { key: 'draft', label: '待审核' },
  { key: 'published', label: '已发布' },
];

export const Articles = observer(function Articles() {
  const navigate = useNavigate();

  useEffect(() => {
    void articleStore
      .fetchArticles(articleStore.status, 1, articleStore.pageSize)
      .catch((e: Error) => message.error(e.message));
  }, []);

  const onTabChange = (key: string) => {
    const status = key as ArticleStatus | '';
    void articleStore
      .fetchArticles(status, 1, articleStore.pageSize)
      .catch((e: Error) => message.error(e.message));
  };

  return (
    <div>
      <h3 style={{ marginTop: 0 }}>文章管理</h3>
      <Tabs
        activeKey={articleStore.status}
        items={TAB_ITEMS}
        onChange={onTabChange}
        style={{ marginBottom: 4 }}
      />
      <Table<Article>
        rowKey="id"
        loading={articleStore.loading}
        dataSource={articleStore.items}
        pagination={{
          current: articleStore.page,
          pageSize: articleStore.pageSize,
          total: articleStore.total,
          showSizeChanger: true,
          onChange: (page, pageSize) =>
            articleStore
              .fetchArticles(articleStore.status, page, pageSize)
              .catch((e: Error) => message.error(e.message)),
        }}
        columns={[
          {
            title: '标题',
            dataIndex: 'title',
            ellipsis: true,
            render: (text: string, record) => (
              <a onClick={() => navigate(`/articles/${record.id}/edit`)}>{text}</a>
            ),
          },
          {
            title: '标签',
            dataIndex: 'tags',
            width: 220,
            render: (tags: string[] | null) =>
              tags && tags.length ? (
                <>
                  {tags.slice(0, 3).map((t) => (
                    <Tag key={t} color="blue">
                      {t}
                    </Tag>
                  ))}
                  {tags.length > 3 && <Tooltip title={tags.join('、')}>+{tags.length - 3}</Tooltip>}
                </>
              ) : (
                '-'
              ),
          },
          { title: '字数', dataIndex: 'word_count', width: 90 },
          {
            title: '状态',
            dataIndex: 'status',
            width: 96,
            render: (status: ArticleStatus) => (
              <Tag color={STATUS_COLOR[status] || 'default'}>
                {ARTICLE_STATUS_TEXT[status] || status}
              </Tag>
            ),
          },
          {
            title: '发布时间',
            dataIndex: 'published_at',
            width: 160,
            render: (v: string | null) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-'),
          },
          {
            title: '创建时间',
            dataIndex: 'created_at',
            width: 160,
            render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-'),
          },
          {
            title: '操作',
            key: 'action',
            width: 100,
            render: (_, record) => (
              <Button
                type="link"
                size="small"
                icon={<EditOutlined />}
                onClick={() => navigate(`/articles/${record.id}/edit`)}
              >
                编辑
              </Button>
            ),
          },
        ]}
      />
    </div>
  );
});
