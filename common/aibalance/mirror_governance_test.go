// mirror_governance_test.go - mirror 内存治理回归测试
//
// 覆盖本次卡死事故的止血主因修复:
//   - 队列容量钳制 (clampMirrorQueueSize / startRuleLocked)
//   - 全局在途字节预算 (超额 drop, 内存有界)
//   - 单字段截断 (prepareSnapshotForQueue 保留尾部闭合标记)
//
// 关键词: mirror 队列钳制回归, 字节预算 drop 回归, 字段截断回归

package aibalance

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

// TestClampMirrorQueueSize 单测钳制边界.
func TestClampMirrorQueueSize(t *testing.T) {
	assert.Equal(t, mirrorDefaultQueueSize, clampMirrorQueueSize(0))
	assert.Equal(t, mirrorDefaultQueueSize, clampMirrorQueueSize(-5))
	assert.Equal(t, 1, clampMirrorQueueSize(1))
	assert.Equal(t, 5, clampMirrorQueueSize(5))
	assert.Equal(t, mirrorMaxQueueSize, clampMirrorQueueSize(1024))
	assert.Equal(t, mirrorMaxQueueSize, clampMirrorQueueSize(mirrorMaxQueueSize+1))
}

// TestMirrorQueueSizeClamped 配置 1024 的规则实际队列容量被钳到 256.
func TestMirrorQueueSizeClamped(t *testing.T) {
	m := NewMirrorManager()
	rule := &schema.AiMirrorRule{
		Name:          "clamp",
		Enabled:       true,
		ConditionType: MirrorConditionAlways,
		Concurrency:   1,
		QueueSize:     1024,
	}
	rule.ID = 7001
	m.startRule(rule)
	defer m.RemoveRule(rule.ID)

	st := m.GetStatus(rule.ID)
	require.NotNil(t, st)
	assert.Equal(t, mirrorMaxQueueSize, st.QueueCapacity,
		"queue capacity must be clamped to mirrorMaxQueueSize")
}

// TestMirrorGlobalByteBudgetDrops 超大快照在小字节预算下被 drop, 内存有界.
func TestMirrorGlobalByteBudgetDrops(t *testing.T) {
	m := NewMirrorManager()
	m.SetMaxInFlightBytes(2 * 1024 * 1024) // 2MB budget

	rule := &schema.AiMirrorRule{
		Name:          "budget",
		Enabled:       true,
		ConditionType: MirrorConditionAlways,
		Concurrency:   1,
		QueueSize:     mirrorMaxQueueSize,
	}
	rule.ID = 7002

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := &mirrorRuleRuntime{
		rule:   rule,
		ch:     make(chan *MirrorSnapshot, mirrorMaxQueueSize),
		cancel: cancel,
		logs:   newMirrorLogRing(),
	}
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		<-ctx.Done() // 永不消费, 制造最坏扣押
	}()
	m.mu.Lock()
	m.runtime[rule.ID] = rt
	m.mu.Unlock()

	// 每个快照 ~1MB, 预算 2MB, 投递 10 个 -> 入队约 2 个, 其余按字节预算 drop.
	for i := 0; i < 10; i++ {
		m.Trigger(makeBigSnapshot("b"+itoa(i), 1024*1024))
	}

	assert.LessOrEqual(t, m.InFlightBytes(), m.MaxInFlightBytes(),
		"in-flight bytes must stay within global budget")

	logs := rt.logs.snapshot()
	drops := 0
	for _, l := range logs {
		if strings.Contains(l.ErrorMessage, "byte budget") {
			drops++
		}
	}
	assert.Greater(t, drops, 0, "expected some byte-budget drops under tight budget")

	cancel()
	rt.wg.Wait()
}

// TestPrepareSnapshotForQueue_TruncatesAndKeepsTail 超大字段被截断且保留尾部.
func TestPrepareSnapshotForQueue_TruncatesAndKeepsTail(t *testing.T) {
	const marker = "</aive_record_json>"
	big := makeBigText(mirrorSnapshotFieldCap+100000) + marker
	snap := &MirrorSnapshot{
		ReqID:        "trunc",
		ResponseText: makeBigText(mirrorSnapshotFieldCap + 50000),
		RequestMessages: []aispec.ChatDetail{
			{Role: "user", Content: big},
		},
	}
	out := prepareSnapshotForQueue(snap)
	require.NotNil(t, out)
	// 原始 snapshot 不被污染 (浅拷贝).
	assert.Equal(t, mirrorSnapshotFieldCap+100000+len(marker), len(snap.RequestMessages[0].Content.(string)))
	// 截断后的 content 不超过上限, 且保留尾部闭合标记.
	got := out.RequestMessages[0].Content.(string)
	assert.LessOrEqual(t, len(got), mirrorSnapshotFieldCap)
	assert.True(t, strings.HasSuffix(got, marker), "truncation must keep the closing marker at tail")
	assert.LessOrEqual(t, len(out.ResponseText), mirrorSnapshotFieldCap)
}
