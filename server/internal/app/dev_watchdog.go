package app

// 开发环境自愈 watchdog：air 在 Windows 下偶发「重编译成功、但旧进程没被杀掉」，
// 结果新二进制已落盘、端口上跑的还是旧代码（本项目反复踩中）。
// 本进程定期比对自己的启动时间与可执行文件的修改时间：若磁盘上的二进制比本进程新，
// 说明自己就是那个没被杀掉的孤儿，主动退出让出端口；air 在下次文件变更时即可正常接管。
// 仅 debug 模式启动：生产 release 没有 air，不存在此问题。

import (
	"os"
	"time"

	"go.uber.org/zap"
)

const (
	watchdogInterval = 3 * time.Second // 轮询二进制 mtime 的间隔
	watchdogGrace    = 5 * time.Second // 宽限：过滤落盘/时钟的毫秒级抖动
)

// startStaleBinaryWatchdog 启动孤儿进程自检 goroutine；仅 debug 模式调用。
func startStaleBinaryWatchdog(log *zap.Logger) {
	exe, err := os.Executable()
	if err != nil {
		log.Warn("自愈 watchdog 无法定位可执行文件，跳过", zap.Error(err))
		return
	}
	boot := time.Now()
	go func() {
		for {
			time.Sleep(watchdogInterval)
			info, err := os.Stat(exe)
			if err != nil {
				continue // 文件暂时不可读（可能正在重编译落盘），下轮再看
			}
			if info.ModTime().After(boot.Add(watchdogGrace)) {
				log.Warn("磁盘上的二进制比本进程新：air 重编译但未能重启本进程（Windows 已知问题），孤儿进程主动退出",
					zap.Time("boot", boot),
					zap.Time("build", info.ModTime()),
					zap.String("exe", exe))
				os.Exit(0)
			}
		}
	}()
}
