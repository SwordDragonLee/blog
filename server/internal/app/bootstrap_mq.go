package app

import (
	"context"

	"blog/server/internal/mq"
	"blog/server/internal/task"

	"go.uber.org/zap"
)

// initMQ 初始化 RabbitMQ 连接、发布者与生成任务消费者。
func (a *App) initMQ(ctx context.Context) error {
	broker := mq.New(a.cfg.RabbitMQ.URL, a.log)
	mqCtx, cancel := context.WithTimeout(ctx, mqBootstrapTimeout)
	_, err := broker.Connect(mqCtx)
	cancel()
	if err != nil {
		// 启动时不因 MQ 不可用而退出：Publisher / Consumer 均会按需重连
		a.log.Warn("RabbitMQ 暂不可用，将在使用时自动重连", zap.Error(err))
	}
	a.broker = broker

	// 发布者（生产者）：往 MQ 投递任务消息 {"task_id":N} 的一方，
	a.publisher = mq.NewPublisher(broker)

	// 消费者：后台 worker，从队列 blog.task.generate 取出消息，
	a.consumer = mq.NewConsumer(broker, a.cfg.Task, a.log, a.pipeline.Handle)

	// OnDead：消息重试超限、即将进入死信队列前的回调——
	// 把任务标为 failed 并同步 Redis 进度，让管理台立刻看到「多次重试仍失败」。
	// 回调依赖 db/rdb，故用闭包包装后注入，mq 包无需感知数据库。
	a.consumer.OnDead = newOnDead(a.db, a.rdb, a.log)

	// Logf：消费者往「任务执行日志」追加一行的回调（mq 包不依赖 task/redis，解耦注入）。
	// 用于重投等消费者自身动作的任务侧留痕，管理台轮询任务详情时可见；
	// task.AppendLog 内部复用进度日志的写入逻辑（RPush、截断 200 条、7 天 TTL）。
	a.consumer.Logf = func(ctx context.Context, taskID uint, format string, args ...any) {
		task.AppendLog(a.rdb, taskID, format, args...)
	}
	a.log.Info("RabbitMQ 已初始化", zap.Int("concurrency", max(a.cfg.Task.Concurrency, 1)))
	return nil
}
