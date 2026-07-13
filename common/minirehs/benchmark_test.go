package minirehs

import (
	"fmt"
	"math/rand"
	"testing"
)

// benchScanRecords 对给定 backend 编译 patterns, 每个 op 把全部 records 各扫描一遍
// (即逐条 HTTP 流量匹配, 对齐真实 MITM 打标场景), 报告聚合吞吐.
func benchScanRecords(b *testing.B, patterns []Pattern, backend BackendKind, records [][]byte) {
	b.Helper()
	db, err := Compile(patterns, WithBackend(backend), WithLogger(silentLogger{}))
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	defer db.Close()
	sc, err := db.NewScratch()
	if err != nil {
		b.Fatalf("scratch: %v", err)
	}
	defer sc.Close()

	var total int64
	for _, r := range records {
		total += int64(len(r))
	}

	b.SetBytes(total)
	b.ReportAllocs()
	b.ResetTimer()
	var hits int64
	for i := 0; i < b.N; i++ {
		cnt := 0
		for _, rec := range records {
			_ = db.Scan(rec, sc, func(m Match) bool {
				cnt++
				return true
			})
		}
		hits += int64(cnt)
	}
	b.StopTimer()
	_ = hits
}

// benchScanRecordsOpts 同 benchScanRecords, 但允许追加任意编译选项 (如 WithReportLocation(false)),
// 用于度量"纯存在性"等不同语义档的吞吐.
func benchScanRecordsOpts(b *testing.B, patterns []Pattern, records [][]byte, opts ...Option) {
	b.Helper()
	allOpts := append([]Option{WithLogger(silentLogger{})}, opts...)
	db, err := Compile(patterns, allOpts...)
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	defer db.Close()
	sc, err := db.NewScratch()
	if err != nil {
		b.Fatalf("scratch: %v", err)
	}
	defer sc.Close()

	var total int64
	for _, r := range records {
		total += int64(len(r))
	}
	b.SetBytes(total)
	b.ReportAllocs()
	b.ResetTimer()
	var hits int64
	for i := 0; i < b.N; i++ {
		cnt := 0
		for _, rec := range records {
			_ = db.Scan(rec, sc, func(m Match) bool {
				cnt++
				return true
			})
		}
		hits += int64(cnt)
	}
	b.StopTimer()
	_ = hits
}

func benchScanRecordsBatchOpts(b *testing.B, patterns []Pattern, records [][]byte, opts ...Option) {
	b.Helper()
	allOpts := append([]Option{WithLogger(silentLogger{})}, opts...)
	db, err := Compile(patterns, allOpts...)
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	defer db.Close()
	sc, err := db.NewScratch()
	if err != nil {
		b.Fatalf("scratch: %v", err)
	}
	defer sc.Close()
	var total int64
	for _, rec := range records {
		total += int64(len(rec))
	}
	b.SetBytes(total)
	b.ReportAllocs()
	b.ResetTimer()
	var hits int64
	for i := 0; i < b.N; i++ {
		cnt := 0
		if err := db.ScanBatch(records, sc, func(_ int, _ Match) bool {
			cnt++
			return true
		}); err != nil {
			b.Fatal(err)
		}
		hits += int64(cnt)
	}
	b.StopTimer()
	_ = hits
}

// BenchmarkMVSExistence 度量"仅判存在性"(WithReportLocation(false), 契合 MITM 打标只需"哪些规则
// 命中"的场景)下 MVS 的吞吐: 走纯位运算快路径、不做 findAllLoc 定位. 带 -tags minirehs_mvs 时
// MVS 存在性热路径改由纯 C99 内核执行 (per-pattern + 合并 always-on).
//
// 不再跑 StdlibLoop 子基准 (每次重测 stdlib 逐条扫描太慢, 价值低). 加速比以"一次测定的固定参照"
// 折算: 本机 StdlibLoop ≈ 0.17 MB/s (87 条逐条扫整段), 故 80x 目标 ≈ 13.6 MB/s. 需复测 stdlib
// 基线时用 BenchmarkMITMRealTraffic/StdlibLoop 单独跑.
//
// 关键词: benchmark, MVS, 存在性, WithReportLocation, MITM 打标, 固定参照 0.17MB/s
func BenchmarkMVSExistence(b *testing.B) {
	patterns, _ := compilableMITMPatterns(b)
	records, joined := loadCorpusB(b)
	b.Logf("rules: %d, records: %d, corpus bytes: %d", len(patterns), len(records), len(joined))
	b.Run("MVS_Exist", func(b *testing.B) {
		benchScanRecordsOpts(b, patterns, records, WithBackend(BackendMVS), WithReportLocation(false))
	})
	b.Run("MVS_Located", func(b *testing.B) {
		benchScanRecordsOpts(b, patterns, records, WithBackend(BackendMVS), WithReportLocation(true))
	})
	// MVS_Exist_RE2only: 仅取 RE2 可直接编译的子集 (排除 regexp2-origin 的 gate 成员). 与 MVS_Exist
	// 之差 = Phase 1 后"超集门 + regexp2 复核"的残余成本 (而非旧的 always-on regexp2 全量税).
	b.Run("MVS_Exist_RE2only", func(b *testing.B) {
		re2 := re2OnlyMITMPatterns(b)
		benchScanRecordsOpts(b, re2, records, WithBackend(BackendMVS), WithReportLocation(false))
	})
	b.Run("MVS_Exist_RE2only_Batch2", func(b *testing.B) {
		re2 := re2OnlyMITMPatterns(b)
		benchScanRecordsBatchOpts(b, re2, records, WithBackend(BackendMVS), WithReportLocation(false))
	})
}

// BenchmarkMITMRealTraffic 用 rule4yak 真实规则集 + 本地库真实流量, 对比:
//   - Engine: minirehs 自研引擎 (一次扫描 + 字面量预过滤)
//   - StdlibLoop: 现状方案 (N 条正则逐条扫描整段数据), 即 "300 正则一次匹配" 的等价基线
//
// 仅使用 RE2 可表达的规则: backref/lookaround 这类 regexp2-only 规则是非线性回溯, 在任何
// 引擎上都同样慢, 且对两侧都计入 always-on, 会掩盖多正则加速的真实收益, 故性能头条排除.
// 它们的正确承载已在一致性测试中覆盖 (TestConsistencyMITMRealTraffic, 含全部 87 条).
//
// 关键词: benchmark, MITM, 真实流量, stdlib 逐条对照, compile then scan
func BenchmarkMITMRealTraffic(b *testing.B) {
	patterns := re2OnlyMITMPatterns(b)
	records, joined := loadCorpusB(b)
	b.Logf("RE2 rules used: %d, records: %d, corpus bytes: %d", len(patterns), len(records), len(joined))

	b.Run("Engine", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendEngine, records)
	})
	b.Run("StdlibLoop", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendStdlib, records)
	})
}

// re2OnlyMITMPatterns 返回标准库 RE2 可直接编译的 MITM 规则 (排除 regexp2-only 与非法规则).
func re2OnlyMITMPatterns(b *testing.B) []Pattern {
	all, _ := mitmPatterns(b)
	var out []Pattern
	for _, p := range all {
		if _, _, err := compileAndParse(buildExprWithFlags(p)); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// genSyntheticPatterns 生成 n 条字面量丰富的代表性正则 (各带唯一字面量锚点).
func genSyntheticPatterns(n int) []Pattern {
	out := make([]Pattern, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Pattern{
			ID:   PatternID(i + 1),
			Expr: fmt.Sprintf("tok%05dz[0-9a-f]{4,8}", i),
		})
	}
	return out
}

// genSyntheticRecords 生成一组小报文 (模拟逐条流量), 稀疏植入少量 pattern 字面量.
func genSyntheticRecords(n, recSize, count int) [][]byte {
	r := rand.New(rand.NewSource(42))
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789 /.:=\"'\n"
	records := make([][]byte, 0, count)
	for c := 0; c < count; c++ {
		buf := make([]byte, 0, recSize)
		for len(buf) < recSize {
			if r.Intn(800) == 0 {
				id := r.Intn(n)
				buf = append(buf, fmt.Sprintf("tok%05dz%08x", id, r.Uint32())...)
				continue
			}
			buf = append(buf, alphabet[r.Intn(len(alphabet))])
		}
		records = append(records, buf[:recSize])
	}
	return records
}

// BenchmarkSyntheticScale 展示 "扫一遍 vs 扫 N 遍" 的优势随 pattern 数 N 放大:
// 固定约 1 MB 语料 (256 条 4 KB 报文), pattern 数 N 从 50 增到 1000.
func BenchmarkSyntheticScale(b *testing.B) {
	const recSize = 4096
	const count = 256 // 共约 1 MB
	for _, n := range []int{50, 100, 300, 500, 1000} {
		patterns := genSyntheticPatterns(n)
		records := genSyntheticRecords(n, recSize, count)
		b.Run(fmt.Sprintf("N=%d/Engine", n), func(b *testing.B) {
			benchScanRecords(b, patterns, BackendEngine, records)
		})
		b.Run(fmt.Sprintf("N=%d/StdlibLoop", n), func(b *testing.B) {
			benchScanRecords(b, patterns, BackendStdlib, records)
		})
	}
}

type silentLogger struct{}

func (silentLogger) Infof(string, ...interface{})  {}
func (silentLogger) Warnf(string, ...interface{})  {}
func (silentLogger) Errorf(string, ...interface{}) {}
func (silentLogger) Debugf(string, ...interface{}) {}

// 下面两个 B 版辅助函数复用 testing.TB 版逻辑 (testing.B 满足 testing.TB).
func compilableMITMPatternsB(b *testing.B) ([]Pattern, map[PatternID]string) {
	return compilableMITMPatterns(b)
}

func loadCorpusB(b *testing.B) ([][]byte, []byte) {
	return loadCorpus(b)
}
