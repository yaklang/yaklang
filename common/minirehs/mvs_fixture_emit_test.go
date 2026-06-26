package minirehs

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestMVSEmitKernelFixture 导出 "真实规则编出的 C 内核 blob + 真实流量输入 + Go 参考期望" 三件套,
// 供独立 C 校验器 (native/mvscan/amalgamation/fixture_check.c) 在 ASan/UBSan 下跑真实 NFA 执行,
// 做单文件发行件的内存安全 + 真实负载正确性护栏 (无需 cgo / 无需 Go asan 全量重编).
//
// 默认跳过 (仅当 MVS_EMIT_FIXTURE=1 时产出, 写到 MVS_FIXTURE_DIR, 缺省 /tmp/mvs_fixture).
//
// 文件格式 (全部小端):
//
//	blob.bin     原始 C 内核 blob (buildMVSBlob 产出, 含 per-pattern + 合并 always-on NFA)
//	inputs.bin   u32 count; 每条 u32 len + len 字节 (语料各记录 + joined 整段)
//	expected.txt 三行: npat / 合并命中累计 totalMerged / 存在性命中累计 totalExists
//
// totalMerged = sum over inputs of |merged.scanExist(in)|
// totalExists = sum over inputs of #{idx: nfas[idx]!=nil && !hasAssert && existsIn(in)}
// (即 C blob 可执行的非断言 NFA 集合, 与 C mvscan_db_nfa_exists 可判定集合一致.)
//
// 关键词: mvscan, amalgamation, fixture, ASan, UBSan, real traffic, drift guard
// digMVSDB 从 Database 挖出内部 *mvsDB (与 getMVSDB 同逻辑, 但本文件无构建标签, 默认构建即可跑;
// getMVSDB 定义在 cgo 标签文件里, 默认构建不可见).
func digMVSDB(t *testing.T, db Database) *mvsDB {
	t.Helper()
	d, ok := db.(*database)
	if !ok {
		t.Fatalf("db is not *database: %T", db)
	}
	c, ok := d.primary.(*compositeDB)
	if !ok {
		t.Fatalf("primary is not *compositeDB: %T", d.primary)
	}
	m, ok := c.primary.(*mvsDB)
	if !ok {
		t.Fatalf("composite.primary is not *mvsDB: %T", c.primary)
	}
	return m
}

func TestMVSEmitKernelFixture(t *testing.T) {
	if os.Getenv("MVS_EMIT_FIXTURE") != "1" {
		t.Skip("set MVS_EMIT_FIXTURE=1 to emit C kernel fixture for sanitizer harness")
	}
	outDir := os.Getenv("MVS_FIXTURE_DIR")
	if outDir == "" {
		outDir = filepath.Join(os.TempDir(), "mvs_fixture")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}

	patterns, _ := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer db.Close()
	mdb := digMVSDB(t, db)

	blob := buildMVSBlob(mdb.nfas, mdb.merged)
	if len(blob) == 0 {
		t.Fatal("empty blob")
	}

	records, joined := loadCorpus(t)
	inputs := append(append([][]byte{}, records...), joined)

	// C 可执行的非断言 NFA 下标集合.
	var cEligible []int
	for idx, nfa := range mdb.nfas {
		if nfa != nil && !nfa.hasAssert {
			cEligible = append(cEligible, idx)
		}
	}

	totalMerged := 0
	totalExists := 0
	for _, in := range inputs {
		if mdb.merged != nil {
			seen := make([]bool, mdb.n)
			hits := mdb.merged.scanExist(in, seen, nil)
			totalMerged += len(hits)
		}
		for _, idx := range cEligible {
			if mdb.nfas[idx].existsIn(in) {
				totalExists++
			}
		}
	}

	// 写 blob.bin
	if err := os.WriteFile(filepath.Join(outDir, "blob.bin"), blob, 0o644); err != nil {
		t.Fatalf("write blob: %v", err)
	}
	// 写 inputs.bin
	var ib []byte
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], uint32(len(inputs)))
	ib = append(ib, u32[:]...)
	for _, in := range inputs {
		binary.LittleEndian.PutUint32(u32[:], uint32(len(in)))
		ib = append(ib, u32[:]...)
		ib = append(ib, in...)
	}
	if err := os.WriteFile(filepath.Join(outDir, "inputs.bin"), ib, 0o644); err != nil {
		t.Fatalf("write inputs: %v", err)
	}
	// 写 expected.txt
	exp := fmt.Sprintf("%d\n%d\n%d\n", mdb.n, totalMerged, totalExists)
	if err := os.WriteFile(filepath.Join(outDir, "expected.txt"), []byte(exp), 0o644); err != nil {
		t.Fatalf("write expected: %v", err)
	}

	// 同时落一份 C 可执行下标列表 (便于排错; 校验器不强依赖).
	sort.Ints(cEligible)
	var el []byte
	for _, idx := range cEligible {
		el = append(el, []byte(fmt.Sprintf("%d\n", idx))...)
	}
	_ = os.WriteFile(filepath.Join(outDir, "eligible.txt"), el, 0o644)

	t.Logf("fixture emitted to %s: blob=%d bytes, inputs=%d, npat=%d, totalMerged=%d, totalExists=%d",
		outDir, len(blob), len(inputs), mdb.n, totalMerged, totalExists)
}
