package ssaapi

import (
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

// TestVulUploadBuildPerformance 验证 vul_upload.go 的 Build 耗时应小于 100ms（优化后预期 ~26ms）
func TestVulUploadBuildPerformance(t *testing.T) {
	vulinboxPath := findVulinboxPath()
	if vulinboxPath == "" {
		t.Skip("vulinbox path not found, skip performance test")
	}
	// 确保 vul_upload.go 存在
	vulUploadPath := filepath.Join(vulinboxPath, "vul_upload.go")
	if !utils.FileExists(vulUploadPath) {
		t.Skip("vul_upload.go not found, skip performance test")
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

	prog := progs[0]
	config := prog.GetConfig()
	require.NotNil(t, config, "program config should not be nil")

	recorder := config.GetFilePerformanceRecorder()
	require.NotNil(t, recorder, "file performance recorder should not be nil")

	snapshots := recorder.Snapshot()
	var vulUploadBuild *struct {
		Name  string
		Total time.Duration
	}
	for _, m := range snapshots {
		if strings.HasPrefix(m.Name, "Build[") && strings.Contains(m.Name, "vul_upload.go") {
			vulUploadBuild = &struct {
				Name  string
				Total time.Duration
			}{m.Name, m.Total}
			break
		}
	}

	if vulUploadBuild == nil {
		buildNames := make([]string, 0, len(snapshots))
		for _, m := range snapshots {
			if strings.HasPrefix(m.Name, "Build[") {
				buildNames = append(buildNames, m.Name)
			}
		}
		t.Fatalf("Build[vul_upload.go] not found in performance snapshot; Build names: %v", buildNames)
	}

	maxBuildTime := 100 * time.Millisecond
	require.Less(t, vulUploadBuild.Total, maxBuildTime,
		"vul_upload.go Build time %v should be < %v (optimization regression)", vulUploadBuild.Total, maxBuildTime)
}
