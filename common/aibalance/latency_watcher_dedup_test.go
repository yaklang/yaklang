// latency_watcher_dedup_test.go - latency watcher 在途去重回归测试
//
// 覆盖抗坍塌修复: 同一坏 provider 多 tick 只允许一个在途健康检查, 避免重复
// go triggerHealthCheck 叠加大量卡在上游的 goroutine.
//
// 关键词: latency watcher 在途去重回归, tryStartCheck/finishCheck

package aibalance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLatencyWatcherInFlightDedup 同一 provider 在途时重复抢占失败, 释放后可再抢.
func TestLatencyWatcherInFlightDedup(t *testing.T) {
	w := NewLatencyWatcher()

	assert.True(t, w.tryStartCheck(42), "first acquire should succeed")
	assert.False(t, w.tryStartCheck(42), "second acquire while in-flight should be deduped")

	// 不同 provider 互不影响.
	assert.True(t, w.tryStartCheck(43), "different provider should acquire independently")

	// 释放后可再次抢占.
	w.finishCheck(42)
	assert.True(t, w.tryStartCheck(42), "after finishCheck, re-acquire should succeed")

	// 清理.
	w.finishCheck(42)
	w.finishCheck(43)
	assert.Equal(t, 0, len(w.inFlightChecks))
}
