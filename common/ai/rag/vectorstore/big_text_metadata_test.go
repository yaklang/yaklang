package vectorstore

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

// 关键词: chunk_size metadata, type lenient, processBigText metadata override
// 这个测试验证 doc.Metadata["chunk_size"] 在 int / int64 / float64 三种类型下
// 都能被 processBigText 正确识别，避免 Yak 脚本传 2000 时因类型断言失败而走默认值。
func TestProcessBigText_ChunkSizeMetadataIsTypeLenient(t *testing.T) {
	const expected = 2000

	cases := []struct {
		name string
		raw  any
	}{
		{name: "int", raw: int(expected)},
		{name: "int64", raw: int64(expected)},
		{name: "float64", raw: float64(expected)},
		{name: "uint", raw: uint(expected)},
		{name: "string", raw: "2000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := utils.InterfaceToInt(tc.raw)
			if got != expected {
				t.Fatalf("InterfaceToInt(%T %v) = %d, want %d", tc.raw, tc.raw, got, expected)
			}
		})
	}
}

// TestProcessBigText_ChunkOverlapMetadataIsTypeLenient 验证 chunk_overlap 同样宽容
func TestProcessBigText_ChunkOverlapMetadataIsTypeLenient(t *testing.T) {
	const expected = 200

	cases := []struct {
		name string
		raw  any
	}{
		{name: "int", raw: int(expected)},
		{name: "int64", raw: int64(expected)},
		{name: "float64", raw: float64(expected)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := utils.InterfaceToInt(tc.raw)
			if got != expected {
				t.Fatalf("InterfaceToInt(%T %v) = %d, want %d", tc.raw, tc.raw, got, expected)
			}
		})
	}
}
