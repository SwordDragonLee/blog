// Package mq 封装 RabbitMQ：连接管理、拓扑声明、任务消息的发布与消费
// （失败重投 + 死信队列）。
package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"

	"go.uber.org/zap"
)

// 拓扑常量：direct exchange blog.tasks → 队列 blog.task.generate，
// 死信 exchange blog.tasks.dlx → 队列 blog.task.dlq。
const (
	ExchangeTasks = "blog.tasks"
	ExchangeDLX   = "blog.tasks.dlx"
	QueueGenerate = "blog.task.generate"
	QueueDLQ      = "blog.task.dlq"
	RoutingKey    = "task.generate"

	// HeaderRetryCount 消费失败重试计数，随重投消息透传。
	HeaderRetryCount = "x-retry-count"
)

// TaskMessage 任务消息体：{"task_id":N}。
type TaskMessage struct {
	TaskID uint `json:"task_id"`
}

// DeclareTopology 幂等地声明 exchange、队列与绑定（可重复调用）。
func DeclareTopology(ch *amqp091.Channel) error {
	if err := ch.ExchangeDeclare(ExchangeTasks, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("声明 exchange %s: %w", ExchangeTasks, err)
	}
	if err := ch.ExchangeDeclare(ExchangeDLX, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("声明 exchange %s: %w", ExchangeDLX, err)
	}
	// 生成任务队列：持久化，消费端 nack 且不重投时经 DLX 落入死信队列
	if _, err := ch.QueueDeclare(QueueGenerate, true, false, false, false, amqp091.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}); err != nil {
		return fmt.Errorf("声明队列 %s: %w", QueueGenerate, err)
	}
	if _, err := ch.QueueDeclare(QueueDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("声明队列 %s: %w", QueueDLQ, err)
	}
	if err := ch.QueueBind(QueueGenerate, RoutingKey, ExchangeTasks, false, nil); err != nil {
		return fmt.Errorf("绑定队列 %s: %w", QueueGenerate, err)
	}
	if err := ch.QueueBind(QueueDLQ, RoutingKey, ExchangeDLX, false, nil); err != nil {
		return fmt.Errorf("绑定队列 %s: %w", QueueDLQ, err)
	}
	return nil
}

// MQ 管理与 RabbitMQ 的共享连接：懒建立，断开后在使用处按需重建。
type MQ struct {
	url string
	log *zap.Logger

	mu   sync.Mutex
	conn *amqp091.Connection
}

// New 创建 MQ 管理器（不立即建连）。
func New(url string, log *zap.Logger) *MQ {
	return &MQ{url: url, log: log}
}

// Connect 返回可用连接；未连接或已断开时重建并声明拓扑。
func (m *MQ) Connect(ctx context.Context) (*amqp091.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conn != nil && !m.conn.IsClosed() {
		return m.conn, nil
	}
	conn, err := amqp091.Dial(m.url)
	if err != nil {
		return nil, fmt.Errorf("连接 RabbitMQ: %w", err)
	}
	ch, err := conn.Channel()
	if err == nil {
		err = DeclareTopology(ch)
		_ = ch.Close()
	}
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("初始化 MQ 拓扑: %w", err)
	}
	m.conn = conn
	m.log.Info("已连接 RabbitMQ")
	return conn, nil
}

// Channel 从共享连接开一个新 channel；连接恰好断开时强制重连一次。
func (m *MQ) Channel(ctx context.Context) (*amqp091.Channel, error) {
	conn, err := m.Connect(ctx)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err == nil {
		return ch, nil
	}
	m.mu.Lock()
	if m.conn == conn {
		m.conn = nil
	}
	m.mu.Unlock()
	if conn, err = m.Connect(ctx); err != nil {
		return nil, err
	}
	return conn.Channel()
}

// Close 关闭底层连接。
func (m *MQ) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conn != nil {
		_ = m.conn.Close()
		m.conn = nil
	}
}

// marshalTask 序列化任务消息体。
func marshalTask(taskID uint) ([]byte, error) {
	return json.Marshal(TaskMessage{TaskID: taskID})
}

// publishing 构造持久化任务消息。
func publishing(body []byte, headers amqp091.Table) amqp091.Publishing {
	if headers == nil {
		headers = amqp091.Table{}
	}
	return amqp091.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp091.Persistent,
		Timestamp:    time.Now(),
		Headers:      headers,
		Body:         body,
	}
}
