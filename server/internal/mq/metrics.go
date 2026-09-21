package mq

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// MQ 消费/投递核心指标（经 router 暴露在 GET /metrics）。
// 命名前缀 blog_mq_，result 标签区分消息终局：
//
//	ack     处理成功确认
//	requeue 停机/重投失败等场景重新入队
//	discard 任务已取消，确认丢弃
//	retry   失败后带计数重投
//	dead    重试超限或非法消息，转入死信队列
var (
	metricConnectTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "blog_mq_connect_total",
		Help: "与 RabbitMQ 成功建立连接的次数（含首次与断线重连）",
	})
	metricPublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "blog_mq_publish_total",
		Help: "任务消息投递结果计数",
	}, []string{"result"}) // ok / error
	metricConsumedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "blog_mq_consumed_total",
		Help: "消息消费终局计数",
	}, []string{"result"}) // ack / requeue / discard / retry / dead
	metricConsumeDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "blog_mq_consume_duration_seconds",
		Help:    "单条消息处理耗时（生成任务为分钟级，桶位相应放宽）",
		Buckets: []float64{.1, .5, 1, 5, 15, 60, 180, 600},
	})
	metricInflight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "blog_mq_inflight",
		Help: "当前正在处理（已投递未确认）的消息数",
	})
)
