package coreplugin

import (
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func Check(code string, t *testing.T) {
	prog := ssaapi.Parse(code, plugin_type_analyzer.GetPluginSSAOpt("mitm")...)
	if prog.IsNil() {
		t.Fatal("Failed to parse code")
	}
	if res := yak.AnalyzeStaticYaklangWithType(string(code), "yak"); len(lo.Filter(res, func(item *yak.StaticAnalyzeResult, index int) bool {
		return item.Severity == "error"
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
		if !file.IsDir() {
			filePath := fmt.Sprintf("base-yak-plugin/%s", file.Name())
			codeBytes, err := basePlugin.ReadFile(filePath)
			if err != nil {
				log.Error("Failed to read file:", err)
				continue
			}

			t.Run(fmt.Sprintf("plugin %s", file.Name()), func(t *testing.T) {
				Check(string(codeBytes), t)
			})
		}
	}
}
