package mustpass

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
)

func copyDir(srcDir string, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(dstDir, 0755)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			srcData, err := os.ReadFile(srcPath)
			if err != nil {
				return fmt.Errorf("failed to read source file %s: %w", srcPath, err)
			}
			if err := os.WriteFile(dstPath, srcData, 0644); err != nil {
				return fmt.Errorf("failed to write destination file %s: %w", dstPath, err)
			}
		}
	}
	return nil
}

var files = make(map[string]string)     // 通用测试文件
var filesHids = make(map[string]string) // HIDS 特定测试文件

var vulinboxAddr string
var testDir string

func init() {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to get current file path")
	}
	baseDir := filepath.Dir(filename)

	// 加载通用测试文件
	filesDir := filepath.Join(baseDir, "files")
	if entries, err := os.ReadDir(filesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yak") {
				continue
			}
			filePath := filepath.Join(filesDir, entry.Name())
			raw, err := os.ReadFile(filePath)
			if err != nil {
				panic(fmt.Sprintf("failed to read file %s: %v", filePath, err))
			}
			files[entry.Name()] = string(raw)
		}
	}

	// 加载 HIDS 特定测试文件
	filesHidsDir := filepath.Join(baseDir, "files-hids")
	if entries, err := os.ReadDir(filesHidsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yak") {
				continue
			}
			filePath := filepath.Join(filesHidsDir, entry.Name())
			raw, err := os.ReadFile(filePath)
			if err != nil {
				panic(fmt.Sprintf("failed to read file %s: %v", filePath, err))
			}
			filesHids[entry.Name()] = string(raw)
		}
	}

	// 复制 HIDS 测试数据目录
	srcTestDir := filepath.Join(baseDir, "test-hids")
	if _, err := os.Stat(srcTestDir); err == nil {
		tmpDir := os.TempDir()
		testDir = filepath.Join(tmpDir, "mustpass-hids-test")
		if _, err := os.Stat(testDir); err == nil {
			os.RemoveAll(testDir)
		}
		if err := copyDir(srcTestDir, testDir); err != nil {
			panic(fmt.Sprintf("failed to copy test directory: %v", err))
		}
	}

	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()
	yak.NewScriptEngine(1)

	var err error
	vulinboxAddr, err = vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic("VULINBOX START ERROR")
	}
}

// TestMustPass 运行通用测试用例
func TestMustPass(t *testing.T) {
	yakit.RegisterLowHTTPSaveCallback()

	var cases [][]string
	for k, v := range files {
		cases = append(cases, []string{k, v})
	}

	sort.SliceStable(cases, func(i, j int) bool {
		return cases[i][0] < cases[j][0]
	})

	if vulinboxAddr == "" {
		panic("VULINBOX START ERROR")
	}

	for _, i := range cases {
		caseName, caseContent := i[0], i[1]
		t.Run(caseName, func(t *testing.T) {
			t.Parallel()

			vars := map[string]interface{}{
				"VULINBOX":      vulinboxAddr,
				"VULINBOX_HOST": utils.ExtractHostPort(vulinboxAddr),
			}

			var pcapdbManager *pcapdb.InstanceManager
			if caseName == "pcapdb.yak" {
				// A private profile/catalog avoids changing global YAKIT_HOME or
				// retaining captures in the user's library during parallel tests.
				root := t.TempDir()
				profile, err := gorm.Open("sqlite3", filepath.Join(root, "profile.sqlite"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { profile.Close() })
				manager, err := pcapdb.NewInstanceManager(profile, filepath.Join(root, "library"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { manager.Close() })
				vars["PCAPDB_TEST_EXPORTS"] = pcapdb.ExportsWithManager(context.Background(), manager)
				pcapdbManager = manager
			}

			_, err := yak.Execute(caseContent, vars)
			if err != nil {
				t.Fatalf("run script[%s] error: %v", caseName, err)
			}
			if pcapdbManager != nil {
				count, err := pcapdbManager.Count()
				if err != nil || count != 1 {
					t.Fatalf("pcapdb smoke did not use its isolated library: count=%d, err=%v", count, err)
				}
			}
		})
	}
}

// TestMustPassHIDS 运行 HIDS 特定测试用例
func TestMustPassHIDS(t *testing.T) {
	if !hidsBuildEnabled {
		t.Skip("HIDS mustpass tests require the hids build tag")
	}

	yakit.RegisterLowHTTPSaveCallback()

	var cases [][]string
	for k, v := range filesHids {
		cases = append(cases, []string{k, v})
	}

	sort.SliceStable(cases, func(i, j int) bool {
		return cases[i][0] < cases[j][0]
	})

	if testDir == "" {
		t.Skip("HIDS test directory not available")
	}

	for _, i := range cases {
		caseName, caseContent := i[0], i[1]
		t.Run(caseName, func(t *testing.T) {
			// HIDS 测试不并行运行，避免文件系统冲突
			vars := map[string]interface{}{
				"VULINBOX":      vulinboxAddr,
				"VULINBOX_HOST": utils.ExtractHostPort(vulinboxAddr),
				"TEST_DIR":      testDir,
			}

			_, err := yak.Execute(caseContent, vars)
			if err != nil {
				t.Fatalf("run script[%s] error: %v", caseName, err)
			}
		})
	}
}
