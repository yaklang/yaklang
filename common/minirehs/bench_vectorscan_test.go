//go:build minirehs_vectorscan

// 本基准把本引擎 (纯 Go, RE2 精确偏移) 与 Vectorscan 后端 (单一 SIMD 自动机, 存在性匹配)
// 在同一规则集 + 同一真实语料上, 经由同一个 Scan API 逐条 block 扫描做对照。
//
// 运行 (需运行时可加载 libhs, 如 brew install vectorscan):
//
//	CGO_ENABLED=1 go test -tags minirehs_vectorscan ./common/minirehs/ \
//	    -run '^$' -bench 'BenchmarkEngineVsVectorscan' -benchtime 10x
//
// 关键词: vectorscan, hyperscan, baseline, 成熟系统对照, 存在性匹配
package minirehs

import "testing"

// BenchmarkEngineVsVectorscan 在同一 (各自可编译的) MITM 规则集 + 同一真实语料上,
// 对照 StdlibLoop / 本引擎 / Vectorscan 后端三者的逐条 block 扫描吞吐。
func BenchmarkEngineVsVectorscan(b *testing.B) {
	patterns := re2OnlyMITMPatterns(b)
	records, _ := loadCorpusB(b)
	b.Logf("rules: %d, records: %d, libhs: %s", len(patterns), len(records), hsVersion())

	b.Run("StdlibLoop", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendStdlib, records)
	})
	b.Run("Engine", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendEngine, records)
	})
	b.Run("Vectorscan", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendVectorscan, records)
	})
}

// BenchmarkVectorscanFullRuleset 用全部可被各自后端编译的 MITM 规则 (含 regexp2-only,
// 由 fallback 承载), 展示 Vectorscan 后端在真实打标场景的端到端吞吐。
func BenchmarkVectorscanFullRuleset(b *testing.B) {
	patterns, _ := compilableMITMPatternsB(b)
	records, _ := loadCorpusB(b)
	b.Logf("compilable rules: %d, records: %d", len(patterns), len(records))

	b.Run("Engine", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendEngine, records)
	})
	b.Run("Vectorscan", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendVectorscan, records)
	})
}
