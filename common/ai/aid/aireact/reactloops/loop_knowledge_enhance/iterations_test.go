package loop_knowledge_enhance

import "testing"

// TestClampKnowledgeEnhanceIterations 验证迭代次数 clamp 逻辑:
// 显式 1-10 生效, 其它(<=0 或 >10, 含全局默认 100)回退 defaultIterations(2).
func TestClampKnowledgeEnhanceIterations(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{in: 0, want: 2},   // 未设置 -> 默认 2
		{in: -5, want: 2},  // 非法 -> 默认 2
		{in: 1, want: 1},   // rag-server 默认一次迭代
		{in: 2, want: 2},   // 显式小值生效
		{in: 10, want: 10}, // 边界生效
		{in: 11, want: 2},  // 超出范围 -> 默认 2
		{in: 100, want: 2}, // 全局默认 -> 保持原有 2 轮行为
	}
	for _, c := range cases {
		if got := clampKnowledgeEnhanceIterations(c.in); got != c.want {
			t.Fatalf("clampKnowledgeEnhanceIterations(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
