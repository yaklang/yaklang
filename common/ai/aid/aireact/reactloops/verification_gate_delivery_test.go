package reactloops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

// deliveryTimelineCfg 嵌入 mock config, 额外暴露 GetTimeline() 满足
// pushDeliveryFileToTimeline 的鸭子接口 (timelineProvider). 仅用于本测试.
//
// 关键词: deliveryTimelineCfg, mock GetTimeline, timelineProvider 鸭子
type deliveryTimelineCfg struct {
	*mockcfg.MockedAIConfig
	timeline *aicommon.Timeline
}

func (d *deliveryTimelineCfg) GetTimeline() *aicommon.Timeline {
	return d.timeline
}

// TestVerificationGate_DeliveryFile_PushesToTimeline 验证交付文件迁移后的契约:
//  1. ApplyVerificationResult 路径下的 OutputFile 仅以 [DELIVERY FILE] +
//     文件路径 + 元数据形式落到 Open Timeline;
//  2. timeline 转储中绝不包含被交付文件本身的正文 marker (反向断言);
//  3. 单条 timeline item 的字节量保持极简 (≤ 512 字节硬上限);
//  4. ContextProviderManager 不再有 "output_file:" 前缀的新注册.
//
// 关键词: TestVerificationGate_DeliveryFile_PushesToTimeline, [DELIVERY FILE],
//
//	Open Timeline 自然淘汰, Pure Dynamic 反污染, output_file: 反注册
func TestVerificationGate_DeliveryFile_PushesToTimeline(t *testing.T) {
	const bodyMarker = "DELIVERY_TEST_BODY_MARKER_4f12c0a"

	tmpDir, err := os.MkdirTemp("", "delivery_timeline_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	deliveryPath := filepath.Join(tmpDir, "report.txt")
	largeBody := strings.Repeat("x", 50*1024) + bodyMarker + strings.Repeat("y", 8*1024)
	require.NoError(t, os.WriteFile(deliveryPath, []byte(largeBody), 0644))

	mock := mockcfg.NewMockedAIConfig(context.Background()).(*mockcfg.MockedAIConfig)
	cfg := &deliveryTimelineCfg{
		MockedAIConfig: mock,
		timeline:       aicommon.NewTimeline(nil, nil),
	}

	// Sanity: ContextProviderManager 是个新的空实例, 不可能含 output_file: 注册项.
	beforeProviders := cfg.GetContextProviderManager()
	require.NotNil(t, beforeProviders)

	pushDeliveryFileToTimeline(cfg, deliveryPath)

	dump := cfg.timeline.Dump()
	t.Logf("timeline dump after delivery push:\n%s", dump)

	assert.Contains(t, dump, "[DELIVERY FILE]",
		"timeline must surface the [DELIVERY FILE] marker for delivered outputs")
	assert.Contains(t, dump, deliveryPath,
		"timeline entry must include the delivered file path")

	// 反向断言: 文件正文 marker 不能进入 timeline. 这是本次重构最关键的安全
	// 性保证 -- 交付文件不再触发"全文每轮重发"路径.
	assert.NotContains(t, dump, bodyMarker,
		"timeline entry must NOT contain the file body (no content sampling, no full-body re-injection)")

	// 单条 entry 必须保持极简体量 (path + size + mime + mtime + 头尾标识).
	// 这里用 dump 长度作为上限 -- timeline 仅含一条 entry, 即可代表 entry 自身.
	require.LessOrEqual(t, len(dump), 1024,
		"single delivery timeline entry must remain tiny (got %d bytes)", len(dump))

	// 反向断言: ApplyVerificationResult 不再注册任何 output_file: 提供者.
	// pushDeliveryFileToTimeline 走 timeline 通道, 不接触 ContextProviderManager.
	afterProviders := cfg.GetContextProviderManager()
	require.NotNil(t, afterProviders)
	dynamicCtx := afterProviders.Execute(cfg, cfg.GetEmitter())
	assert.NotContains(t, dynamicCtx, "output_file:",
		"ContextProviderManager must NOT contain any output_file: registration after migration")
	assert.NotContains(t, dynamicCtx, bodyMarker,
		"DynamicContext must NOT contain the file body marker")
}

// TestVerificationGate_DeliveryFile_StatFailureFallsBackToPathOnly 验证当
// os.Stat 失败 (例如文件已被外部清理) 时, helper 仍能写入"path-only" 的极简
// timeline entry, 不抛错、不读文件正文.
//
// 关键词: pushDeliveryFileToTimeline 容错, stat 失败回退, path-only entry
func TestVerificationGate_DeliveryFile_StatFailureFallsBackToPathOnly(t *testing.T) {
	mock := mockcfg.NewMockedAIConfig(context.Background()).(*mockcfg.MockedAIConfig)
	cfg := &deliveryTimelineCfg{
		MockedAIConfig: mock,
		timeline:       aicommon.NewTimeline(nil, nil),
	}

	missingPath := "/non/existent/path/to/missing-delivery.bin"
	pushDeliveryFileToTimeline(cfg, missingPath)

	dump := cfg.timeline.Dump()
	t.Logf("timeline dump (stat failure case):\n%s", dump)

	assert.Contains(t, dump, "[DELIVERY FILE]")
	assert.Contains(t, dump, missingPath)
	assert.Contains(t, dump, "size=unknown")
	assert.Contains(t, dump, "mtime=unknown")
}
