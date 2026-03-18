package ssaapi

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
)

func findVulinboxPath() string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for d := dir; d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			p := filepath.Join(d, "common", "vulinbox")
			if _, err := os.Stat(p); err == nil {
				return p
			}
			break
		}
	}
	return utils.GetFirstExistedPath("common/vulinbox", "./common/vulinbox")
}

// golangScanRuleNames 用于 vulinbox 扫描的 3 个 Golang 规则名（与 rule_versions.json 一致）
var golangScanRuleNames = []string{
	"检测Golang不安全的证书验证",
	"检测Golang命令注入漏洞",
	"检测Golang中使用Sprig的SSTI漏洞",
}

// TestVulinboxBuildPerformance 编译 vulinbox 并执行 3 个 Golang 规则扫描，校验流程完成
func TestVulinboxBuildPerformance(t *testing.T) {
	vulinboxPath := findVulinboxPath()
	if vulinboxPath == "" {
		t.Skip("vulinbox path not found, skip performance test")
	}

	diagnostics.SetLevel(diagnostics.LevelHigh)
	progs, err := ssaapi.ParseProjectFromPath(vulinboxPath,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithProgramName("vulinbox-perf-test"),
		ssaapi.WithReCompile(true),
		ssaapi.WithConcurrency(1),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progs[0].GetProgramName())

	require.NotNil(t, progs[0].GetConfig().DiagnosticsRecorder(), "recorder should not be nil")

	err = syntaxflow_scan.StartScan(context.Background(),
		ssaconfig.WithProgramNames(progs[0].GetProgramName()),
		ssaconfig.WithRuleFilterLanguage("golang"),
		ssaconfig.WithRuleFilterRuleNames(golangScanRuleNames...),
	)
	require.NoError(t, err)
}

// TestDebugBuildPerformance 编译 go2ssa 目录，仅本地运行（CI 跳过）
func TestDebugBuildPerformance(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip debug build test on GitHub/CI")
	}

	_, testFile, _, _ := runtime.Caller(0)
	absDir, err := filepath.Abs(filepath.Join(filepath.Dir(testFile), "..", "..", "..", "go2ssa"))
	if err != nil {
		t.Skipf("resolve path: %v", err)
	}
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		t.Skipf("go2ssa dir not found: %s", absDir)
	}

	diagnostics.SetLevel(diagnostics.LevelHigh)
	progs, err := ssaapi.ParseProjectFromPath(absDir,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithMemory(),
		ssaapi.WithProgramName("debug-build-perf"),
		ssaapi.WithReCompile(true),
		ssaapi.WithConcurrency(1),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progs[0].GetProgramName())

	rec := progs[0].GetConfig().DiagnosticsRecorder()
	require.NotNil(t, rec)
	snap := rec.Snapshot()
	t.Logf("compiled %s: %d measurements", absDir, len(snap))
}
