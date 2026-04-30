package aireact

import "testing"

// 关键词: compression score threshold, passesCompressionScore, 0.3
// 验证 RAG 知识压缩阶段的评分阈值：旧阈值 0.4 偏严，会把含目标 API 名
// 但「未触及权威定义」的 inline 用法示例片段过滤掉；新阈值 0.3 让这类
// 「弱相关但有线索价值」的内容也能进入最终结果。
func TestPassesCompressionScore_Boundaries(t *testing.T) {
	cases := []struct {
		name  string
		score float64
		want  bool
	}{
		{name: "well above threshold", score: 0.85, want: true},
		{name: "core relevance", score: 0.60, want: true},
		{name: "supplementary info", score: 0.45, want: true},
		// 关键回归断言：0.35 落在新阈值 [0.30, 0.40) 之间，应该保留
		{name: "weak signal kept under new threshold", score: 0.35, want: true},
		// 边界：恰好等于阈值时保留
		{name: "exactly at threshold", score: 0.30, want: true},
		// 略低于阈值时过滤
		{name: "just below threshold", score: 0.29, want: false},
		{name: "noise", score: 0.10, want: false},
		{name: "zero", score: 0.0, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := passesCompressionScore(tc.score); got != tc.want {
				t.Fatalf("passesCompressionScore(%.2f) = %v, want %v", tc.score, got, tc.want)
			}
		})
	}
}

// TestCompressionScoreThreshold_LockedAt0_3 锁定阈值常量，避免误改回 0.4
// 如果业务确实需要调整阈值，请同步更新本测试与 prompt 中的评分语义说明。
func TestCompressionScoreThreshold_LockedAt0_3(t *testing.T) {
	if compressionScoreThreshold != 0.3 {
		t.Fatalf("compressionScoreThreshold expected 0.3, got %.2f; "+
			"若确需调整请同步更新 invoke_compress_long_text_with_dest.go 的 prompt 评分标准段",
			compressionScoreThreshold)
	}
}
