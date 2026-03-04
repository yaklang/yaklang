package ssaapi

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// findVulinboxPath 从模块根目录查找 common/vulinbox
func findVulinboxPath() string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	// 从 test/golang 向上一层层找 go.mod 所在目录
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

// vulinboxBuildPerfCase 定义单个文件的 Build 性能校验规则
type vulinboxBuildPerfCase struct {
	file         string        // 文件名（如 vul_upload.go）
	maxBuildTime time.Duration // 最大允许 Build 耗时
	targetDesc   string        // 目标描述，用于错误信息
}

// TestVulinboxBuildPerformance 一次性编译 vulinbox，校验多个关键文件的 Build 耗时（合并原 TestVulUpload、TestVulXss）
func TestVulinboxBuildPerformance(t *testing.T) {
	vulinboxPath := findVulinboxPath()
	if vulinboxPath == "" {
		t.Skip("vulinbox path not found, skip performance test")
	}

	cases := []vulinboxBuildPerfCase{
		{"vul_upload.go", 100 * time.Millisecond, "vul_upload 优化后 ~26ms"},
		{"vul_xss.go", 3 * time.Second, "vul_xss 优化目标 < 500ms，当前 ~1.7s，阈值防回退"},
	}

	// 至少有一个待测文件存在才编译
	hasAnyFile := false
	for _, c := range cases {
		if utils.FileExists(filepath.Join(vulinboxPath, c.file)) {
			hasAnyFile = true
			break
		}
	}
	if !hasAnyFile {
		t.Skip("no vulinbox perf target files found, skip performance test")
	}

	progs, err := ssaapi.ParseProjectFromPath(vulinboxPath,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithFilePerformanceLog(true),
		ssaapi.WithMemory(),
		ssaapi.WithProgramName("vulinbox-perf-test"),
		ssaapi.WithReCompile(true),
		ssaapi.WithConcurrency(1),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs, "expected at least one program")

	recorder := progs[0].GetConfig().GetFilePerformanceRecorder()
	require.NotNil(t, recorder, "file performance recorder should not be nil")

	snapshots := recorder.Snapshot()

	// 收集 Build 名称，用于失败时的诊断
	buildNames := make([]string, 0, len(snapshots))
	for _, m := range snapshots {
		if strings.HasPrefix(m.Name, "Build[") {
			buildNames = append(buildNames, m.Name)
		}
	}

	for _, c := range cases {
		if !utils.FileExists(filepath.Join(vulinboxPath, c.file)) {
			t.Logf("skip %s: file not found", c.file)
			continue
		}

		var found *struct {
			Name  string
			Total time.Duration
		}
		for _, m := range snapshots {
			if strings.HasPrefix(m.Name, "Build[") && strings.Contains(m.Name, c.file) {
				found = &struct {
					Name  string
					Total time.Duration
				}{m.Name, m.Total}
				break
			}
		}

		if found == nil {
			t.Errorf("Build[%s] not found in snapshot; Build names: %v", c.file, buildNames)
			continue
		}

		require.Less(t, found.Total, c.maxBuildTime,
			"%s Build time %v should be < %v (optimization regression, %s)", c.file, found.Total, c.maxBuildTime, c.targetDesc)
	}
}

// TestDebugBuildPerformance 硬编码目录 go2ssa 的性能测试，仅在本地运行（GitHub CI 自动跳过）
func TestDebugBuildPerformance(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" {
		t.Skip("skip debug build test on GitHub/CI")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Skipf("getwd failed: %v", err)
	}
	// 从 common/yak/ssaapi/test/golang 向上一级到 yak，再进 go2ssa
	absDir, err := filepath.Abs(filepath.Join(cwd, "..", "..", "..", "go2ssa"))
	if err != nil {
		t.Skipf("resolve path failed: %v", err)
	}
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		t.Skipf("build dir not found: %s", absDir)
	}

	progs, err := ssaapi.ParseProjectFromPath(absDir,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithFilePerformanceLog(true),
		ssaapi.WithMemory(),
		ssaapi.WithProgramName("debug-build-perf"),
		ssaapi.WithReCompile(true),
		ssaapi.WithConcurrency(1),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs, "expected at least one program")

	recorder := progs[0].GetConfig().GetFilePerformanceRecorder()
	require.NotNil(t, recorder, "file performance recorder should not be nil")

	snapshots := recorder.Snapshot()
	t.Logf("compiled %s: %d measurements", absDir, len(snapshots))
	for _, m := range snapshots {
		line := fmt.Sprintf("  %s: %v", m.Name, m.Total)
		if m.Size > 0 && m.Total > 0 {
			ratio := float64(m.Total.Milliseconds()) / (float64(m.Size) / 1024)
			line += fmt.Sprintf(" | %.2f ms/KB", ratio)
		}
		t.Log(line)
	}
}
