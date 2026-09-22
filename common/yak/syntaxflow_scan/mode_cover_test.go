package syntaxflow_scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	_ "github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const coverStructRule = `desc(
	mode: "struct"
	language: "php"
	title: "结构扫描：mysql_query"
	level: high
	risk: "sql-injection"
)
mysql_query(* as $arg) as $call
alert $call for { level: "high", risk: "sql-injection" }
`

const coverSSARule = `desc(
	mode: "ssa"
	language: "php"
	title: "数据流：mysql_query"
	level: high
	risk: "sql-injection"
)
mysql_query(*?{!opcode: const && !opcode: param} as $dyn) as $call
alert $dyn for { level: "high", risk: "sql-injection" }
`

func TestScanProject_StructVersusStructAndSSACover(t *testing.T) {
	code := "<?php\nfunction bad() { mysql_query($_GET[\"id\"]); }\nfunction ok() { mysql_query(\"select 1\"); }\n"
	structOnly := scanCoverModes(t, code, syntaxflow_scan.StructMode)
	both := scanCoverModes(t, code, syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode)

	var structModes, bothModes []string
	for _, risk := range structOnly {
		structModes = append(structModes, risk.ScanMode+":"+risk.Title)
	}
	for _, risk := range both {
		bothModes = append(bothModes, risk.ScanMode+":"+risk.Title)
	}
	t.Logf("struct only: %v", structModes)
	t.Logf("struct+ssa: %v", bothModes)

	require.NotEmpty(t, structOnly, "struct scan should hit mysql_query")
	for _, risk := range structOnly {
		require.Equal(t, "struct", risk.ScanMode)
	}

	require.NotEmpty(t, both)
	var ssaCount, structCount int
	for _, risk := range both {
		switch risk.ScanMode {
		case "ssa":
			ssaCount++
		case "struct":
			structCount++
		default:
			t.Fatalf("unexpected scan mode %s", risk.ScanMode)
		}
	}
	require.Greater(t, ssaCount, 0, "ssa rule should hit the external SQL")
	require.Less(t, len(both), len(structOnly)+ssaCount, "ssa result should cover the struct hit on the same call")
	require.Greater(t, structCount, 0, "constant SQL should stay on the struct rule")
}

func scanCoverModes(t *testing.T, code string, modes ...string) []*riskView {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.php"), []byte(code), 0o644))
	program := "cover-scan-" + utils.RandStringBytes(8)
	t.Cleanup(func() {
		db := ssadb.GetDB()
		_ = yakit.DeleteSSARisks(db, &ypb.SSARisksFilter{ProgramName: []string{program}})
		ssadb.DeleteProgram(db, program)
	})

	opts := []ssaconfig.Option{
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("php"),
		ssaconfig.WithSetProgramName(program),
		syntaxflow_scan.WithMode(modes...),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Content: coverStructRule, Language: "php"}),
		ssaconfig.WithScanIgnoreLanguage(true),
	}
	for _, mode := range modes {
		if mode == syntaxflow_scan.SSAMode {
			opts = append(opts, ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Content: coverSSARule, Language: "php"}))
		}
	}
	_, err := syntaxflow_scan.ScanProject(context.Background(), opts...)
	require.NoError(t, err)

	_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{program},
	}, nil)
	require.NoError(t, err)
	out := make([]*riskView, 0, len(risks))
	for _, risk := range risks {
		if risk == nil {
			continue
		}
		out = append(out, &riskView{ScanMode: risk.ScanMode, Title: risk.Title, Line: risk.Line})
	}
	return out
}

type riskView struct {
	ScanMode string
	Title    string
	Line     int64
}

// TestScanProject_DVWAStructAndSSA compares mode=struct with mode=struct+ssa
// on a local DVWA tree. It is not part of the default suite.
func TestScanProject_DVWAStructAndSSA(t *testing.T) {
	if os.Getenv("YAK_SCAN_DVWA") == "" {
		t.Skip("set YAK_SCAN_DVWA=1 to scan /home/wlz/Target/DVWA")
	}
	root := "/home/wlz/Target/DVWA"
	info, err := os.Stat(root)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	var inputs []*ypb.SyntaxFlowRuleInput
	err = filepath.Walk(filepath.Join("..", "..", "syntaxflow", "sfbuildin", "buildin", "php"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".sf" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		inputs = append(inputs, &ypb.SyntaxFlowRuleInput{Content: string(raw), Language: "php"})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, inputs)

	summarize := func(modes ...string) map[string]int {
		program := "dvwa-cover-" + utils.RandStringBytes(6)
		t.Cleanup(func() {
			db := ssadb.GetDB()
			_ = yakit.DeleteSSARisks(db, &ypb.SSARisksFilter{ProgramName: []string{program}})
			ssadb.DeleteProgram(db, program)
		})
		opts := []ssaconfig.Option{
			ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
			ssaconfig.WithCodeSourceLocalFile(root),
			ssaconfig.WithProjectRawLanguage("php"),
			ssaconfig.WithSetProgramName(program),
			syntaxflow_scan.WithMode(modes...),
			ssaconfig.WithScanIgnoreLanguage(true),
		}
		for _, input := range inputs {
			opts = append(opts, ssaconfig.WithRuleInput(input))
		}
		_, err := syntaxflow_scan.ScanProject(context.Background(), opts...)
		require.NoError(t, err)
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{program}}, &ypb.Paging{
			Page:  1,
			Limit: 10000,
		})
		require.NoError(t, err)
		counts := map[string]int{}
		for _, risk := range risks {
			if risk == nil {
				continue
			}
			counts[risk.ScanMode]++
		}
		t.Logf("modes %v risks %d by scan mode %v", modes, len(risks), counts)
		return counts
	}

	structCounts := summarize(syntaxflow_scan.StructMode)
	bothCounts := summarize(syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode)
	t.Logf("DVWA struct=%v struct+ssa=%v", structCounts, bothCounts)
	require.NotEmpty(t, structCounts)
	require.Greater(t, bothCounts["ssa"], 0)
}
