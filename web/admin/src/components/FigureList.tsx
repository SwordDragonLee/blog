import { Card, Col, Empty, Row, Spin, Tag, Typography } from 'antd';
import type { Figure } from '../types';
import { figureUrl } from '../types';

const KIND_TEXT: Record<string, { label: string; color: string }> = {
  cover: { label: '封面', color: 'purple' },
  architecture: { label: '架构图', color: 'blue' },
  flow: { label: '流程图', color: 'cyan' },
  compare: { label: '对比', color: 'orange' },
  timeline: { label: '时间线', color: 'green' },
};

export function FigureList({
  figures,
  loading,
}: {
  figures?: Figure[];
  loading?: boolean;
}) {
  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 24 }}>
        <Spin tip="配图加载中..." />
      </div>
    );
  }
  if (!figures || figures.length === 0) {
    return <Empty description="暂无配图" image={Empty.PRESENTED_IMAGE_SIMPLE} />;
  }
  return (
    <Row gutter={[12, 12]}>
      {figures.map((fig) => {
        const kind = KIND_TEXT[fig.kind] || { label: fig.kind, color: 'default' };
        return (
          <Col key={fig.id} xs={24} sm={12} md={8} lg={6}>
            <Card
              size="small"
              cover={
                <img
                  src={figureUrl(fig.id)}
                  alt={fig.title}
                  style={{ width: '100%', background: '#0f172a' }}
                  loading="lazy"
                />
              }
            >
              <Card.Meta
                title={
                  <>
                    <Tag color={kind.color}>{kind.label}</Tag>
                    <Typography.Text style={{ fontSize: 13 }}>{fig.title}</Typography.Text>
                  </>
                }
                description={`#${fig.id}${fig.placeholder ? ` · {{figure:${fig.placeholder}}}` : ''}`}
              />
            </Card>
          </Col>
        );
      })}
    </Row>
  );
}
