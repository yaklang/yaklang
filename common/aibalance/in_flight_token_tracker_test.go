package aibalance

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 关键词: InFlightTokenTracker 单元测试 CRUD clamp 并发

// TestInFlightTokenTracker_AddRemoveGet_Basic 验证 Add → Get → Remove → Get 闭环
// 在单 bucket 上做加减确认账目一致。
// 关键词: InFlightTokenTracker Add Get Remove 闭环
func TestInFlightTokenTracker_AddRemoveGet_Basic(t *testing.T) {
	tracker := NewInFlightTokenTracker()
	assert.Equal(t, int64(0), tracker.Get("bucket-a"), "fresh bucket must be 0")

	tracker.Add("bucket-a", 100)
	assert.Equal(t, int64(100), tracker.Get("bucket-a"))

	tracker.Add("bucket-a", 50)
	assert.Equal(t, int64(150), tracker.Get("bucket-a"))

	tracker.Remove("bucket-a", 70)
	assert.Equal(t, int64(80), tracker.Get("bucket-a"))

	tracker.Remove("bucket-a", 80)
	assert.Equal(t, int64(0), tracker.Get("bucket-a"), "should return 0 after full release")
}

// TestInFlightTokenTracker_MultiBucket_Isolation 验证不同 bucket 完全独立，
// 不会互相串扰（global "" 和 model 独立桶并存的真实场景）。
// 关键词: InFlightTokenTracker 多桶隔离
func TestInFlightTokenTracker_MultiBucket_Isolation(t *testing.T) {
	tracker := NewInFlightTokenTracker()
	tracker.Add("", 200)          // 全局桶
	tracker.Add("model-a", 300)   // 模型 a 独立桶
	tracker.Add("model-b", 1)     // 模型 b 独立桶
	assert.Equal(t, int64(200), tracker.Get(""))
	assert.Equal(t, int64(300), tracker.Get("model-a"))
	assert.Equal(t, int64(1), tracker.Get("model-b"))

	tracker.Remove("model-a", 100)
	assert.Equal(t, int64(200), tracker.Get(""), "global unaffected")
	assert.Equal(t, int64(200), tracker.Get("model-a"))
	assert.Equal(t, int64(1), tracker.Get("model-b"))
}

// TestInFlightTokenTracker_RemoveClampToZero 验证 Remove 多于 Add 时桶被钳到 0
// 不会变负（防止 daily check 算到负的 effective_used）。
// 关键词: InFlightTokenTracker clamp-to-zero, 负数防御
func TestInFlightTokenTracker_RemoveClampToZero(t *testing.T) {
	tracker := NewInFlightTokenTracker()
	tracker.Add("b", 100)
	tracker.Remove("b", 999) // 故意 Remove 多于 Add
	assert.Equal(t, int64(0), tracker.Get("b"), "must clamp to 0, not negative")

	// 再 Add 后状态恢复正常
	tracker.Add("b", 5)
	assert.Equal(t, int64(5), tracker.Get("b"))
}

// TestInFlightTokenTracker_NoopOnZeroOrNegative 验证 Add/Remove 0 或负数为 no-op，
// 不会污染 map 也不会改值。
// 关键词: InFlightTokenTracker 0/负值 no-op
func TestInFlightTokenTracker_NoopOnZeroOrNegative(t *testing.T) {
	tracker := NewInFlightTokenTracker()
	tracker.Add("x", 0)
	tracker.Add("x", -50)
	assert.Equal(t, int64(0), tracker.Get("x"))

	tracker.Add("x", 100)
	tracker.Remove("x", 0)
	tracker.Remove("x", -10)
	assert.Equal(t, int64(100), tracker.Get("x"))
}

// TestInFlightTokenTracker_NilSafety 验证对 nil receiver 的所有方法都 graceful
// 退化，让"未初始化 tracker"的 ServerConfig 不会 NPE（防御性编程）。
// 关键词: InFlightTokenTracker nil safety
func TestInFlightTokenTracker_NilSafety(t *testing.T) {
	var tracker *InFlightTokenTracker
	tracker.Add("x", 100)         // no-op, no panic
	tracker.Remove("x", 100)      // no-op, no panic
	assert.Equal(t, int64(0), tracker.Get("x"))
	snap := tracker.Snapshot()
	assert.NotNil(t, snap)
	assert.Empty(t, snap)
}

// TestInFlightTokenTracker_Snapshot 验证 Snapshot 返回独立拷贝、内部改动不影响
// portal 已经拿到的快照。
// 关键词: InFlightTokenTracker Snapshot 独立拷贝
func TestInFlightTokenTracker_Snapshot(t *testing.T) {
	tracker := NewInFlightTokenTracker()
	tracker.Add("a", 10)
	tracker.Add("b", 20)
	snap := tracker.Snapshot()
	assert.Equal(t, int64(10), snap["a"])
	assert.Equal(t, int64(20), snap["b"])

	tracker.Add("a", 5)
	// snap 是独立拷贝，不受后续 Add 影响
	assert.Equal(t, int64(10), snap["a"], "snapshot must be a deep copy")
	assert.Equal(t, int64(15), tracker.Get("a"), "tracker itself updated")
}

// TestInFlightTokenTracker_ConcurrentAddRemove 200 个 goroutine 并发 Add/Remove
// 200 次，验证最终账目守恒（净增 == 总 Add - 总 Remove）。
// 关键词: InFlightTokenTracker 并发 goroutine 守恒
func TestInFlightTokenTracker_ConcurrentAddRemove(t *testing.T) {
	tracker := NewInFlightTokenTracker()
	const goroutines = 200
	const opsPerG = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerG; i++ {
				tracker.Add("hot", 7)
				tracker.Remove("hot", 3) // 每轮净 +4
			}
		}()
	}
	wg.Wait()
	expected := int64(goroutines * opsPerG * (7 - 3))
	assert.Equal(t, expected, tracker.Get("hot"),
		"concurrent add/remove must preserve net account (lock correctness)")
}

// TestInFlightTokenTracker_ConcurrentAddOnly 高并发只 Add，验证总和正确（无丢更新）。
// 关键词: InFlightTokenTracker 并发 Add 无丢更新
func TestInFlightTokenTracker_ConcurrentAddOnly(t *testing.T) {
	tracker := NewInFlightTokenTracker()
	const goroutines = 100
	const opsPerG = 500
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerG; i++ {
				tracker.Add("bucket", 1)
			}
		}()
	}
	wg.Wait()
	expected := int64(goroutines * opsPerG)
	assert.Equal(t, expected, tracker.Get("bucket"),
		"concurrent Add: no lost update")
}
