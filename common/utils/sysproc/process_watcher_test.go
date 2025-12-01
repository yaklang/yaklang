package sysproc

import (
	"context"
	"testing"
	"time"
)

func TestProcessesWatcher_Start(t *testing.T) {
	t.Skip("Skip TestProcessesWatcher_Start")
	watcher := NewProcessesWatcher()
	watcher.Start(
		func(ctx context.Context, p *ProcessBasicInfo) {
			t.Logf("Process created: PID %d, Name: %s, Exe: %s", p.Pid, p.Name, p.Exe)
		},
		func(ctx context.Context, p *ProcessBasicInfo) {
			t.Logf("Process exited: PID %d, Name: %s, Exe: %s", p.Pid, p.Name, p.Exe)
		},
		time.Second,
	)
	defer watcher.Stop()

	// 让监控器运行一段时间以观察输出
	time.Sleep(10 * time.Minute)
}
