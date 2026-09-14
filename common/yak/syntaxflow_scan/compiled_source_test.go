package syntaxflow_scan

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestNewSourceQueryTargetFromProgramUsesCompiledSnapshot(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.yak", "key = \"AKIAIOSFODNN7EXAMPLE\"\n")
	progs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithProgramName(t.Name()))
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	src := ssaapi.NewSourceQueryTargetFromProgram(progs[0])
	require.NotEmpty(t, src.Files())
	found := false
	for _, content := range src.Files() {
		if strings.Contains(content, "AKIAIOSFODNN7EXAMPLE") {
			found = true
			break
		}
	}
	require.True(t, found, "compiled snapshot should keep the scanned file content")
}

func TestStartScan_CompiledProgramAlsoRunsSourceRules(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.yak", "key = \"AKIAIOSFODNN7EXAMPLE\"\n")
	progs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithProgramName(t.Name()))
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	var alerts int
	err = StartScan(context.Background(),
		WithPrograms(progs[0]),
		WithCompiledSource(true),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "compiled source snapshot")
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			Language: string(ssaconfig.General),
		}),
		WithScanResultCallback(func(r *ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0, "source rules should run against IrSource/FileList of a compiled program")
}

func TestRuleFilterModesIncludesSourceWhenProgramHasSnapshot(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a.yak", "a = 1\n")
	progs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithProgramName(t.Name()))
	require.NoError(t, err)
	cfg, err := NewConfig(WithPrograms(progs[0]), WithCompiledSource(true))
	require.NoError(t, err)
	attachCompiledSourceTargets(cfg)
	modes := ruleFilterModes(cfg)
	require.Contains(t, modes, string(schema.SFR_MODE_SSA))
	require.Contains(t, modes, string(schema.SFR_MODE_SOURCE))
}
