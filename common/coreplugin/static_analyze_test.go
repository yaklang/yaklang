package coreplugin

import (
	"fmt"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func MITMCheck(code string, t *testing.T) {
	_, err := ssaapi.Parse(code, static_analyzer.GetPluginSSAOpt("mitm")...)
	if err != nil {
		t.Fatal("Failed to parse code: ", err)
	}
	if res := yak.StaticAnalyzeYaklang(string(code), "mitm"); len(lo.Filter(res, func(item *result.StaticAnalyzeResult, index int) bool {
		return item.Severity == result.Error
	})) != 0 {
		t.Fatalf("plugin : static analyzer failed: \n%s", res)
	}
}

func TestAnalyzeMustPASS_CorePlugin(t *testing.T) {
	files, err := basePlugin.ReadDir("base-yak-plugin")
	if err != nil {
		log.Error("Failed to read directory:", err)
		return
	}

	for _, file := range files {
		if strings.Contains(file.Name(), "核心引擎性能采样") {
			continue
		}
		if !file.IsDir() {
			filePath := fmt.Sprintf("base-yak-plugin/%s", file.Name())
			codeBytes, err := basePlugin.ReadFile(filePath)
			if err != nil {
				log.Error("Failed to read file:", err)
				continue
			}

			t.Run(fmt.Sprintf("plugin %s", file.Name()), func(t *testing.T) {
				MITMCheck(string(codeBytes), t)
			})
		}
	}
}
