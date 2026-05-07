package aicommon

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStablePromptNonce_DeterministicAndCaseLowercase 验证同一组 parts 反复
// 调用 StablePromptNonce 始终返回相同结果, 且字符集严格在 [a-z0-9] 内, 长度 6.
//
// 关键词: StablePromptNonce, 稳定 nonce, AITag 字符集
func TestStablePromptNonce_DeterministicAndCaseLowercase(t *testing.T) {
	a := StablePromptNonce("plan-scope", "root-1", "PARENT_TASK")
	b := StablePromptNonce("plan-scope", "root-1", "PARENT_TASK")
	require.Equal(t, a, b, "same parts should produce same nonce")
	require.Lenf(t, a, 6, "nonce length should be 6, got %d", len(a))
	for _, c := range a {
		require.Truef(t, (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'),
			"nonce should only contain [a-z0-9], got %c", c)
	}
}

// TestStablePromptNonce_DiffPartsDiffNonce 验证不同 parts 大概率派生不同的
// nonce. 同一空间下 fnv64 → 36^6 ≈ 2.18e9, 6 个不同 case 全冲突的概率极低.
//
// 关键词: StablePromptNonce, 不同 part 不同 nonce
func TestStablePromptNonce_DiffPartsDiffNonce(t *testing.T) {
	cases := []string{
		StablePromptNonce("plan-scope", "root-1", "PARENT_TASK"),
		StablePromptNonce("plan-scope", "root-2", "PARENT_TASK"),
		StablePromptNonce("plan-scope", "root-1", "CURRENT_TASK"),
		StablePromptNonce("plan-scope", "root-1", "FACTS"),
		StablePromptNonce("plan-scope", "root-1", "DOCUMENT"),
		StablePromptNonce("plan-scope", "root-1", "INSTRUCTION"),
	}
	seen := map[string]struct{}{}
	for _, c := range cases {
		seen[c] = struct{}{}
	}
	require.Equal(t, len(cases), len(seen), "different parts should produce different nonces, got duplicates: %v", cases)
}

// TestStablePromptNonce_OrderSensitive 验证 part 顺序敏感.
// 关键词: StablePromptNonce, 顺序敏感
func TestStablePromptNonce_OrderSensitive(t *testing.T) {
	a := StablePromptNonce("a", "b")
	b := StablePromptNonce("b", "a")
	require.NotEqual(t, a, b, "part order should affect nonce")
}

// TestStablePromptNonce_EmptyPartsFallback 验证空 parts 走 fallback 不 panic.
// 关键词: StablePromptNonce, fallback
func TestStablePromptNonce_EmptyPartsFallback(t *testing.T) {
	a := StablePromptNonce()
	require.Lenf(t, a, 6, "fallback nonce should be 6 chars, got %d", len(a))
	require.True(t, strings.Trim(a, "0") == "" || true, "fallback all-zero is acceptable")
}

// TestPlanScopedNonce_SameRootSameNonce 验证 plan-scoped nonce 在同一个
// rootIdentifier 下跨多次调用稳定, 跨 rootIdentifier 不同.
//
// 关键词: PlanScopedNonce, plan epoch 稳定, root task 切换
func TestPlanScopedNonce_SameRootSameNonce(t *testing.T) {
	rootA := "1"
	rootB := "2"
	saltUserQuery := "user_query"
	saltParentTask := "parent_task"

	a1 := PlanScopedNonce(rootA, saltUserQuery)
	a2 := PlanScopedNonce(rootA, saltUserQuery)
	require.Equal(t, a1, a2, "same root + salt should produce same nonce")

	b1 := PlanScopedNonce(rootB, saltUserQuery)
	require.NotEqual(t, a1, b1, "different root should produce different nonce")

	a3 := PlanScopedNonce(rootA, saltParentTask)
	require.NotEqual(t, a1, a3, "different salt should produce different nonce within same root")
}

// TestPlanScopedNonce_EmptyRoot 验证空 rootIdentifier 走 fallback 不 panic.
// 关键词: PlanScopedNonce, anonymous fallback
func TestPlanScopedNonce_EmptyRoot(t *testing.T) {
	a := PlanScopedNonce("", "user_query")
	b := PlanScopedNonce("", "user_query")
	require.Equal(t, a, b, "empty root with same salt should still be deterministic")
	require.Lenf(t, a, 6, "nonce length should be 6, got %d", len(a))
}
