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

// ErrTaskCanceled 任务被主动取消（或已删除/已取消后被消费）：
// 流水线返回该哨兵错误时，消费者 ack 丢弃消息而不重试。
var ErrTaskCanceled = errors.New("任务已取消")

// Consumer 任务消费者：断线自动重连、按配置并发消费；
// 处理失败时按 x-retry-count 重投（不超过 cfg.Task.MaxRetry），超限进死信队列。
type Consumer struct {
	mq      *MQ
	cfg     config.Task
	log     *zap.Logger
	Handler Handler

	// OnDead 消息重试超限、即将进入死信队列前的回调（用于把任务标记为 failed），可为 nil。
	OnDead func(ctx context.Context, taskID uint)

	// Logf 向任务的实时执行日志追加一行（管理端轮询可见），可为 nil。
	// 用于重投等消费者自身动作的任务侧留痕。
	Logf func(ctx context.Context, taskID uint, format string, args ...any)

	mu      sync.Mutex
	cancels map[uint]context.CancelFunc // 执行中任务的派生 context 取消函数
}

// NewConsumer 创建消费者。handler 为任务流水线入口。
func NewConsumer(mq *MQ, cfg config.Task, log *zap.Logger, handler Handler) *Consumer {
	return &Consumer{mq: mq, cfg: cfg, log: log, Handler: handler,
		cancels: make(map[uint]context.CancelFunc)}
}

// CancelTask 主动取消执行中的任务：取消其派生 context 使流水线尽快中断。
// 任务当前不在执行时为空操作（排队中的任务由 Cancel 接口改状态、消费时跳过）。
func (c *Consumer) CancelTask(taskID uint) {
	c.mu.Lock()
	cancel, ok := c.cancels[taskID]
	c.mu.Unlock()
	if ok {
		cancel()
	}
}

// register / unregister 维护执行中任务的取消函数注册表。
func (c *Consumer) register(taskID uint, cancel context.CancelFunc) {
	c.mu.Lock()
	c.cancels[taskID] = cancel
	c.mu.Unlock()
}

func (c *Consumer) unregister(taskID uint) {
	c.mu.Lock()
	delete(c.cancels, taskID)
	c.mu.Unlock()
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
		// 可中断的重连等待：不用 time.Sleep，是为了停机信号能立即打断等待
		// （ctx 取消则退出 worker），否则等 reconnectDelay 后回到循环顶部重连
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
	start := time.Now()
	metricInflight.Inc()
	defer func() {
		metricInflight.Dec()
		metricConsumeDuration.Observe(time.Since(start).Seconds())
	}()

	var msg TaskMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil || msg.TaskID == 0 {
		c.log.Error("非法任务消息，转入死信队列",
			zap.String("body", string(d.Body)), zap.Error(err))
		c.deadLetter(ctx, ch, d, 0)
		return
	}

	retry := retryCount(d.Headers)
	logger := c.log.With(zap.Uint("task_id", msg.TaskID), zap.Int("retry", retry))

	// 每条消息派生独立 context：服务停机取消父 ctx，主动取消走 CancelTask
	dctx, cancel := context.WithCancel(ctx)
	c.register(msg.TaskID, cancel)
	err := c.safeHandle(dctx, msg.TaskID)
	// 判断「执行期间被 CancelTask 主动取消」必须在释放 cancel 之前——
	// cancel() 本身也会让 dctx.Err() 非 nil，先释放再判断会把一切失败误判为取消
	// （该误判曾导致失败消息被 ack 丢弃、自动重试链从未生效）。
	taskCanceled := dctx.Err() != nil
	cancel()
	c.unregister(msg.TaskID)

	if err == nil {
		metricConsumedTotal.WithLabelValues("ack").Inc()
		_ = d.Ack(false)
		return
	}
	switch {
	case ctx.Err() != nil:
		// 服务关闭导致的失败：不 ack，让 RabbitMQ 重新投递
		logger.Info("服务关闭，消息重新入队", zap.Error(err))
		metricConsumedTotal.WithLabelValues("requeue").Inc()
		_ = d.Nack(false, true)
	case taskCanceled || errors.Is(err, ErrTaskCanceled):
		// 任务被主动取消（执行中断，或取消/删除后才被消费）：
		// 状态已由 Cancel/Delete 接口落库，确认丢弃消息即可
		logger.Info("任务已取消，消息确认丢弃", zap.Error(err))
		metricConsumedTotal.WithLabelValues("discard").Inc()
		_ = d.Ack(false)
	default:
		logger.Error("任务处理失败", zap.Error(err))
		c.retryOrFail(ctx, ch, d, msg.TaskID, retry, err)
	}
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

// retryOrFail 处理失败消息：未超重试上限则带计数重投，超限转入死信队列。
func (c *Consumer) retryOrFail(ctx context.Context, ch *amqp091.Channel, d amqp091.Delivery, taskID uint, retry int, lastErr error) {
	logger := c.log.With(zap.Uint("task_id", taskID), zap.Int("retry", retry))
	if retry < c.cfg.MaxRetry {
		if c.Logf != nil {
			c.Logf(ctx, taskID, "第 %d 次执行失败：%v", retry+1, lastErr)
			c.Logf(ctx, taskID, "将自动重试（重试上限 %d 次），消息已重新入队", c.cfg.MaxRetry)
		}
		if err := c.republish(ctx, ch, taskID, retry+1); err != nil {
			logger.Error("重投失败，消息重新入队", zap.Error(err))
			metricConsumedTotal.WithLabelValues("requeue").Inc()
			_ = d.Nack(false, true)
			return
		}
		logger.Info("已重投任务消息", zap.Int("next_retry", retry+1))
		metricConsumedTotal.WithLabelValues("retry").Inc()
		_ = d.Ack(false)
		return
	}
	// 重试超限：标记失败并进入死信队列
	logger.Error("任务重试超限，转入死信队列")
	c.deadLetter(ctx, ch, d, taskID)
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
	metricConsumedTotal.WithLabelValues("dead").Inc()
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
