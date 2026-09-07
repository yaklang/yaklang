package scannode

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gopsprocess "github.com/shirou/gopsutil/v4/process"
	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	envScriptMaxRSS       = "LEGION_SCRIPT_MAX_RSS_BYTES"
	defaultScriptMaxRSS   = 24 * 1024 * 1024 * 1024
	scriptMemPollInterval = 2 * time.Second
)

// ScriptMemoryLimitError marks a child process cancelled because its RSS
// exceeded the configured guard. The debug artifacts are preserved so the
// pprof snapshot taken before cancellation remains diagnosable.
type ScriptMemoryLimitError struct {
	LimitBytes uint64
	RSSBytes   uint64
}

func (e *ScriptMemoryLimitError) Error() string {
	return fmt.Sprintf(
		"script memory limit exceeded: rss=%dMiB limit=%dMiB",
		e.RSSBytes/1024/1024,
		e.LimitBytes/1024/1024,
	)
}

func contextCancel(task *Task) {
	if task == nil || task.Cancel == nil {
		return
	}
	task.Cancel()
}

// scriptMaxRSSBytes resolves the guard limit from the node environment.
func scriptMaxRSSBytes() uint64 {
	raw := strings.TrimSpace(os.Getenv(envScriptMaxRSS))
	if raw == "" {
		return defaultScriptMaxRSS
	}
	limit, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || limit == 0 {
		return defaultScriptMaxRSS
	}
	return limit
}

// startScriptMemoryGuard cancels a child process before it reaches the host
// OOM killer. The returned stop function is idempotent.
func startScriptMemoryGuard(
	cancel func(),
	pid int,
	exceeded *atomic.Pointer[ScriptMemoryLimitError],
) (stop func(), err error) {
	if pid <= 0 {
		return func() {}, nil
	}

	limit := scriptMaxRSSBytes()
	child, err := gopsprocess.NewProcess(int32(pid))
	if err != nil {
		return nil, utils.Errorf("create script memory guard: %v", err)
	}

	done := make(chan struct{})
	var stopOnce sync.Once
	stop = func() {
		stopOnce.Do(func() { close(done) })
	}

	go func() {
		ticker := time.NewTicker(scriptMemPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
			}
			rss := processTreeRSSBytes(child, map[int32]struct{}{})
			if rss == 0 || rss < limit {
				continue
			}
			limitErr := &ScriptMemoryLimitError{LimitBytes: limit, RSSBytes: rss}
			exceeded.Store(limitErr)
			log.Errorf("[memory-guard] pid=%d rss=%dMiB limit=%dMiB; cancelling child",
				pid, rss/1024/1024, limit/1024/1024)
			terminateProcessTree(child)
			cancel()
			stop()
			return
		}
	}()
	return stop, nil
}

func processTreeRSSBytes(root *gopsprocess.Process, visited map[int32]struct{}) uint64 {
	if root == nil {
		return 0
	}
	if _, seen := visited[root.Pid]; seen {
		return 0
	}
	visited[root.Pid] = struct{}{}
	memory, err := root.MemoryInfo()
	if err != nil || memory == nil {
		return 0
	}
	total := memory.RSS
	children, err := root.Children()
	if err != nil {
		return total
	}
	for _, child := range children {
		total += processTreeRSSBytes(child, visited)
	}
	return total
}

func terminateProcessTree(root *gopsprocess.Process) {
	if root == nil {
		return
	}
	children, err := root.Children()
	if err == nil {
		for _, child := range children {
			terminateProcessTree(child)
		}
	}
	_ = root.Kill()
}
