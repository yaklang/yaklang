package sysproc

import (
	"context"
	"testing"
	"time"
)

func TestConnectionsWatcher(t *testing.T) {
	t.Skip("Skip TestProcessesWatcher_Start")
	pid := 12735
	watcher, err := NewWatcher(int32(pid), func(pid int32, remoteIP string) {
		t.Logf("pid: %d, remoteIP: %s", pid, remoteIP)
	}, time.Second*5)
	if err != nil {
		return
	}

	watcher.Start(context.Background())
}
