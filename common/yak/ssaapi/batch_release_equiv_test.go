package ssaapi

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestBatchReleasePreservesPersistedIR(t *testing.T) {
	without := compileReleaseFixture(t, true)
	with := compileReleaseFixture(t, false)
	require.Equal(t, without.counts, with.counts, "releasing settled values and index dedup keys must not change persisted row counts")
	require.Equal(t, without.offsets, with.offsets, "offset rows must stay the same with the release barrier on")
	require.Equal(t, without.flow, with.flow, "the same syntax query must see the same source values")
}

type releaseFixtureResult struct {
	counts  map[string]int64
	offsets []string
	flow    string
}

func compileReleaseFixture(t *testing.T, disableRelease bool) releaseFixtureResult {
	t.Helper()
	ssa.SetBatchReleaseDisabledForTest(disableRelease)
	t.Cleanup(func() { ssa.SetBatchReleaseDisabledForTest(false) })
	t.Setenv(compileUnitBatchMinFilesEnv, "1")
	t.Setenv(compileUnitBatchMinBytesEnv, "0")
	t.Setenv(compileUnitBatchMaxFilesEnv, "1")

	vf := filesys.NewVirtualFs()
	vf.AddFile("base/Base.java", `package base; class Base { static String value(boolean c) { String x = "parallel-source"; if (c) { x = "left"; } else { x = "right"; } return x; } }`)
	vf.AddFile("top/Top.java", `package top; import base.Base; class Top { void run(boolean c) { println(Base.value(c)); } }`)
	name := uuid.NewString()
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), name) })
	programs, err := ParseProjectWithFS(vf, WithLanguage(ssaconfig.JAVA), WithProgramName(name),
		ssaconfig.WithCompileConcurrency(1), ssaconfig.WithCompileIrCacheTTL(time.Millisecond),
		ssaconfig.WithCompileIrCacheMax(1))
	require.NoError(t, err)
	require.Len(t, programs, 1)
	result, err := programs[0].SyntaxFlowWithError(`println(* #-> * as $source) as $call`)
	require.NoError(t, err)
	flow := result.GetValues("source").String()
	require.Contains(t, flow, "left")
	require.Contains(t, flow, "right")

	db := ssadb.GetDB()
	counts := map[string]int64{}
	for _, model := range []any{&ssadb.IrCode{}, &ssadb.IrOffset{}, &ssadb.IrIndex{}, &ssadb.IrType{}, &ssadb.IrSource{}} {
		var n int64
		require.NoError(t, db.Model(model).Where("program_name = ?", name).Count(&n).Error)
		counts[fmt.Sprintf("%T", model)] = n
	}
	var offsets []ssadb.IrOffset
	require.NoError(t, db.Where("program_name = ?", name).Order("value_id, start_offset, end_offset, variable_name").Find(&offsets).Error)
	rendered := make([]string, 0, len(offsets))
	for _, row := range offsets {
		rendered = append(rendered, fmt.Sprintf("%d:%d:%d:%s", row.ValueID, row.StartOffset, row.EndOffset, row.VariableName))
	}
	sort.Strings(rendered)
	return releaseFixtureResult{counts: counts, offsets: rendered, flow: flow}
}
