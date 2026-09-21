package mq

import (
	"context"
	"fmt"
)

// Publisher 任务消息发布者：向 exchange blog.tasks 投递持久化消息。
type Publisher struct {
	mq *MQ
}

// NewPublisher 创建发布者（复用共享连接）。
func NewPublisher(mq *MQ) *Publisher {
	return &Publisher{mq: mq}
}

// PublishTask 投递一条生成任务消息（routing key task.generate，持久化投递）。
func (p *Publisher) PublishTask(ctx context.Context, taskID uint) error {
	body, err := marshalTask(taskID)
	if err != nil {
		metricPublishTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("序列化任务消息: %w", err)
	}
	ch, err := p.mq.Channel(ctx)
	if err != nil {
		metricPublishTotal.WithLabelValues("error").Inc()
		return err
	}
	defer func() { _ = ch.Close() }()
	if err := ch.PublishWithContext(ctx, ExchangeTasks, RoutingKey, false, false,
		publishing(body, nil)); err != nil {
		metricPublishTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("投递任务 %d 消息: %w", taskID, err)
	}
	metricPublishTotal.WithLabelValues("ok").Inc()
	return nil
}
