package sysproc

import (
	"context"
	"testing"
	"time"
)

func TestAcccc(t *testing.T) {
	pid := 12735
	watcher, err := NewWatcher(int32(pid), func(pid int32, remoteIP string, domain string) {
		t.Logf("pid: %d, remoteIP: %s, domain: %s", pid, remoteIP, domain)
	}, time.Second*5)
	if err != nil {
		return
	}

	watcher.Start(context.Background())
}
