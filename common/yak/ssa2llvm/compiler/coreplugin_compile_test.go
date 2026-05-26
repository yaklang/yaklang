package compiler

import (
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestCorePluginBaseYakPluginsCompileToIR(t *testing.T) {
	if testing.Short() {
		t.Skip("coreplugin compile sweep is slow")
	}

	t.Setenv("YAKIT_HOME", t.TempDir())
	coreplugin.InitDBForTest()

	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Fatal("profile database is not initialized")
	}

	plugins := yakit.QueryYakScriptByIsCore(db, true)
	if len(plugins) == 0 {
		t.Fatal("no coreplugin found in yakit database")
	}

	for _, plugin := range plugins {
		plugin := plugin
		t.Run(plugin.ScriptName, func(t *testing.T) {
			_, comp, _, err := compileInput(plugin.ScriptName, plugin.Content, "yak", nil, "main", nil)
			if comp != nil {
				defer comp.Dispose()
			}
			if err != nil {
				t.Fatalf("compile failed: %v", err)
			}
		})
	}
}
