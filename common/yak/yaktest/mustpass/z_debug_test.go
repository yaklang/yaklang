package mustpass

import (
	"sort"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
)

func TestMustPassDebug(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}

	debugName := "rule_engine_basic.yak"
	var debugCases [][]string

	// 检查通用测试文件
	for k, v := range files {
		if k == debugName {
			debugCases = append(debugCases, []string{k, v})
		}
	}

	// 检查 HIDS 测试文件
	for k, v := range filesHids {
		if k == debugName {
			debugCases = append(debugCases, []string{k, v})
		}
	}

	sort.SliceStable(debugCases, func(i, j int) bool {
		return debugCases[i][0] < debugCases[j][0]
	})

	if vulinboxAddr == "" {
		panic("VULINBOX START ERROR")
	}

	totalTest := t
	for _, i := range debugCases {
		t.Run(i[0], func(t *testing.T) {
			vars := map[string]any{
				"VULINBOX":      vulinboxAddr,
				"VULINBOX_HOST": utils.ExtractHostPort(vulinboxAddr),
			}
			if testDir != "" {
				vars["TEST_DIR"] = testDir
			}

			_, err := yak.Execute(i[1], vars)
			if err != nil {
				t.Fatalf("[%v] error: %v", i[0], err)
				totalTest.FailNow()
			}
		})
	}
}
