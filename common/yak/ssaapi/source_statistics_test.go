package ssaapi_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSourceStatisticsDistinctSourcesAndMissingEvidence(t *testing.T) {
	p := ssaapi.NewTmpProgram("statistics")
	p.Program.Cache = ssa.NewDBCache(nil, p.Program, ssa.ProgramCacheMemory, 2)
	t.Cleanup(p.Program.Cache.CloseWithoutSave)
	p.Program.ExtraFile = map[string]string{}
	for path, content := range map[string]string{"main.yak": "// comment\r\n\r\na=1\n", "empty.yak": ""} {
		editor := memedit.NewMemEditor(content)
		p.Program.SetEditor(path, editor)
		p.Program.FileList[path] = "hash-" + path
		p.Program.ExtraFile[path] = "hash-" + path
	}
	p.Program.LineCount = 999 // cumulative legacy metadata must not be used.
	stats, err := p.GetSourceStatistics()
	require.NoError(t, err)
	require.EqualValues(t, 2, stats.AnalyzedFileCount)
	require.EqualValues(t, 5, stats.AnalyzedLineCount)
	p.Program.FileList["missing.yak"] = "missing"
	stats, err = p.GetSourceStatistics()
	require.Error(t, err)
	require.Nil(t, stats)
	stats, err = ssaapi.NewTmpProgram("empty").GetSourceStatistics()
	require.NoError(t, err)
	require.Zero(t, stats.AnalyzedFileCount)
	require.Zero(t, stats.AnalyzedLineCount)
	p.Program.FileList = nil
	stats, err = p.GetSourceStatistics()
	require.Error(t, err, "missing source registry is not an empty source snapshot")
	require.Nil(t, stats)
}

func TestSourceStatisticsOverlayAndDatabaseReload(t *testing.T) {
	base := "public class A { public int value() { return 1; } }\n"
	changed := "// changed\npublic class A { public int value() { return 2; } }\n"
	keep := "public class Keep {}"
	added := "public class Added {}\n"
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{Files: map[string]string{"A.java": base, "Keep.java": keep, "Delete.java": "public class Delete {}", "config.yml": "setting: true\n"}},
		ssatest.IncrementalStep{
			Files: map[string]string{"A.java": changed, "Added.java": added, "Delete.java": ""},
			Check: func(overlay *ssaapi.ProgramOverLay, _ ssatest.IncrementalCheckStage) {
				baseStats, err := overlay.Base.GetSourceStatistics()
				require.NoError(t, err)
				require.EqualValues(t, 4, baseStats.AnalyzedFileCount)
				require.EqualValues(t, 6, baseStats.AnalyzedLineCount, "a cached base must keep its original source snapshot")
				stats, err := overlay.GetSourceStatistics()
				require.NoError(t, err)
				require.EqualValues(t, 4, stats.AnalyzedFileCount)
				require.EqualValues(t, strings.Count(changed, "\n")+strings.Count(keep, "\n")+strings.Count(added, "\n")+5, stats.AnalyzedLineCount)
			},
		},
	)
}
