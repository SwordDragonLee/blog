package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"blog/server/internal/config"

	"github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	reconnectDelay = 3 * time.Second // 断线重连间隔
)

// Handler 单条任务消息的处理回调。
type Handler func(ctx context.Context, taskID uint) error

// Consumer 任务消费者：断线自动重连、按配置并发消费；
// 处理失败时按 x-retry-count 重投（不超过 cfg.Task.MaxRetry），超限进死信队列。
type Consumer struct {
	mq      *MQ
	cfg     config.Task
	log     *zap.Logger
	Handler Handler

	// OnDead 消息重试超限、即将进入死信队列前的回调（用于把任务标记为 failed），可为 nil。
	OnDead func(ctx context.Context, taskID uint)
}

// NewConsumer 创建消费者。handler 为任务流水线入口。
func NewConsumer(mq *MQ, cfg config.Task, log *zap.Logger, handler Handler) *Consumer {
	return &Consumer{mq: mq, cfg: cfg, log: log, Handler: handler}
}

// Run 阻塞运行直到 ctx 取消；并发度取 cfg.Task.Concurrency（至少 1）。
func (c *Consumer) Run(ctx context.Context) {
	workers := c.cfg.Concurrency
	if workers < 1 {
		workers = 1
	}
	c.log.Info("启动 MQ 消费者", zap.Int("workers", workers), zap.Int("max_retry", c.cfg.MaxRetry))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c.workerLoop(ctx, id)
		}(i)
	}
	wg.Wait()
	c.log.Info("MQ 消费者已退出")
}

// workerLoop 单个消费协程：消费循环退出（断线/拓扑异常）后延迟重连。
func (c *Consumer) workerLoop(ctx context.Context, id int) {
	for {
		if err := c.consume(ctx, id); err != nil && ctx.Err() == nil {
			c.log.Warn("消费循环异常，准备重连", zap.Int("worker", id), zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

// consume 建立消费（独立 channel），阻塞处理消息直到通道关闭或 ctx 取消。
func (c *Consumer) consume(ctx context.Context, id int) error {
	conn, err := c.mq.Connect(ctx)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("打开消费 channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("设置 QoS: %w", err)
	}
	// 幂等重声明，容忍 broker 端拓扑丢失
	if err := DeclareTopology(ch); err != nil {
		return err
	}
	msgs, err := ch.Consume(QueueGenerate, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("订阅队列 %s: %w", QueueGenerate, err)
	}
	c.log.Info("消费者就绪", zap.Int("worker", id))

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-msgs:
			if !ok {
				return errors.New("消息通道已关闭")
			}
			c.handleDelivery(ctx, ch, d)
		}
	}
}

// handleDelivery 处理单条消息：成功 ack；失败按重试次数重投或投递死信。
func (c *Consumer) handleDelivery(ctx context.Context, ch *amqp091.Channel, d amqp091.Delivery) {
	var msg TaskMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil || msg.TaskID == 0 {
		c.log.Error("非法任务消息，转入死信队列",
			zap.String("body", string(d.Body)), zap.Error(err))
		c.deadLetter(ctx, ch, d, 0)
		return
	}

	retry := retryCount(d.Headers)
	logger := c.log.With(zap.Uint("task_id", msg.TaskID), zap.Int("retry", retry))

	if err := c.safeHandle(ctx, msg.TaskID); err != nil {
		if ctx.Err() != nil {
			// 服务关闭导致的失败：不 ack，让 RabbitMQ 重新投递
			logger.Info("服务关闭，消息重新入队", zap.Error(err))
			_ = d.Nack(false, true)
			return
		}
		logger.Error("任务处理失败", zap.Error(err))
		if retry < c.cfg.MaxRetry {
			if err := c.republish(ctx, ch, msg.TaskID, retry+1); err != nil {
				logger.Error("重投失败，消息重新入队", zap.Error(err))
				_ = d.Nack(false, true)
				return
			}
			logger.Info("已重投任务消息", zap.Int("next_retry", retry+1))
			_ = d.Ack(false)
			return
		}
		// 重试超限：标记失败并进入死信队列
		logger.Error("任务重试超限，转入死信队列")
		c.deadLetter(ctx, ch, d, msg.TaskID)
		return
	}
	_ = d.Ack(false)
}

// safeHandle 调用业务 handler，捕获 panic 转为错误。
func (c *Consumer) safeHandle(ctx context.Context, taskID uint) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	return c.Handler(ctx, taskID)
}

// republish 以 retry+1 的重试计数重投一条新的持久化任务消息。
func (c *Consumer) republish(ctx context.Context, ch *amqp091.Channel, taskID uint, nextRetry int) error {
	body, err := marshalTask(taskID)
	if err != nil {
		return err
	}
	return ch.PublishWithContext(ctx, ExchangeTasks, RoutingKey, false, false,
		publishing(body, amqp091.Table{HeaderRetryCount: int64(nextRetry)}))
}

// deadLetter 把原消息投递到死信 exchange（落入 blog.task.dlq）后 ack；
// taskID > 0 时先回调 OnDead 将任务标记为失败。
func (c *Consumer) deadLetter(ctx context.Context, ch *amqp091.Channel, d amqp091.Delivery, taskID uint) {
	if taskID > 0 && c.OnDead != nil {
		c.OnDead(ctx, taskID)
	}
	err := ch.PublishWithContext(ctx, ExchangeDLX, RoutingKey, false, false,
		publishing(d.Body, d.Headers))
	if err != nil {
		c.log.Error("投递死信队列失败", zap.Uint("task_id", taskID), zap.Error(err))
	}
	_ = d.Ack(false)
}

// retryCount 从消息 header 读取重试计数（兼容各整型宽度），缺省为 0。
func retryCount(h amqp091.Table) int {
	if h == nil {
		return 0
	}
	switch v := h[HeaderRetryCount].(type) {
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}
