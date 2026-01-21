package aicommon

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildPromptMemoriesMarkdown_RouteAndFormat(t *testing.T) {
	now := time.Date(2026, 1, 21, 10, 0, 0, 0, time.UTC)

	mustAware := &MemoryEntity{Id: "p1", CreatedAt: now.Add(-24 * time.Hour), Content: "偏好：Go 工具优先 gRPC", P_Score: 0.92, R_Score: 0.80}
	action := &MemoryEntity{Id: "a1", CreatedAt: now.Add(-2 * time.Hour), Content: "处理 TUN 劫持时先检查内存溢出", A_Score: 0.90, R_Score: 0.70, T_Score: 0.60}
	warn := &MemoryEntity{Id: "w1", CreatedAt: now.Add(-72 * time.Hour), Content: "用户 1 月可能提过想搬家", O_Score: 0.20, R_Score: 0.85}
	emotional := &MemoryEntity{Id: "e1", CreatedAt: now.Add(-4 * time.Hour), Content: "提到加班时情绪很消极", E_Score: 0.10, R_Score: 0.60}

	results := []*SearchResult{
		{Entity: mustAware, Score: 0.2},
		{Entity: action, Score: 0.2},
		{Entity: warn, Score: 0.2},
		{Entity: emotional, Score: 0.2},
	}

	md := BuildPromptMemoriesMarkdown(
		results,
		MemoryIntentAdvice,
		WithMemoryInjectNow(now),
		WithMemoryInjectMaxTotal(10),
		WithMemoryInjectMaxPerRoute(10),
		WithMemoryInjectMaxContentRunes(200),
	)

	fmt.Println(md)

	require.Contains(t, md, "### Retrieved Memories (Contextual)")
	require.Contains(t, md, "[ action_tips ]")
	require.Contains(t, md, "经验/可执行提示：处理 TUN 劫持时先检查内存溢出")
	require.Contains(t, md, "[ must_aware ]")
	require.Contains(t, md, "关键偏好/约束：偏好：Go 工具优先 gRPC")
	require.Contains(t, md, "[ reliability_warning ]")
	require.Contains(t, md, "待确认（低可信但高相关）：用户 1 月可能提过想搬家")
}

func TestBuildPromptMemoriesMarkdown_MinUtilityFilters(t *testing.T) {
	now := time.Date(2026, 1, 21, 10, 0, 0, 0, time.UTC)
	low := &MemoryEntity{Id: "x", Content: "noise", R_Score: 0.01}
	results := []*SearchResult{{Entity: low, Score: 0}}

	md := BuildPromptMemoriesMarkdown(
		results,
		MemoryIntentGeneric,
		WithMemoryInjectNow(now),
		WithMemoryInjectMinUtility(0.5),
	)
	require.Empty(t, md)
}
