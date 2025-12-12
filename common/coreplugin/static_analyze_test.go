package coreplugin

import (
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func staticCheck(code, pluginType string, t *testing.T) {
	_, err := ssaapi.Parse(code, static_analyzer.GetPluginSSAOpt(pluginType)...)
	if err != nil {
		t.Fatal("Failed to parse code: ", err)
	}
	if res := yak.StaticAnalyze(string(code), yak.WithStaticAnalyzePluginType(pluginType)); len(lo.Filter(res, func(item *result.StaticAnalyzeResult, index int) bool {
		return item.Severity == result.Error
	})) != 0 {
		t.Fatalf("plugin : static analyzer failed: \n%s", res)
	}
}

func TestAnalyzeMustPASSCorePlugin(t *testing.T) {
	// for i := 0; i < 20; i++ {
	yakit.CallPostInitDatabase()
	for _, plugin := range buildInPlugin {
		t.Run(fmt.Sprintf("plugin(%s) %s", plugin.Type, plugin.ScriptName), func(t *testing.T) {
			staticCheck(plugin.Content, plugin.Type, t)
		})
	}
	// }
}

func TestAnalyzeMustPASSCorePlugin_Debug(t *testing.T) {
	// for i := 0; i < 20; i++ {
	if utils.InGithubActions() {
		t.Skip("Skip in Github Actions")
	}
	target := "SSA 项目探测"
	yakit.CallPostInitDatabase()
	for _, plugin := range buildInPlugin {
		if plugin.ScriptName != target {
			continue
		}
		t.Run(fmt.Sprintf("plugin(%s) %s", plugin.Type, plugin.ScriptName), func(t *testing.T) {
			staticCheck(plugin.Content, plugin.Type, t)
		})
	}
	// }
}
