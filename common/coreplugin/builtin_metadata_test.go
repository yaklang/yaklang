package coreplugin

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/builtinmetadata"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
)

func TestBuiltinPluginMetadataAndUnchangedSourceRefresh(t *testing.T) {
	const name = "HTTP请求走私"
	code := string(GetCorePluginData(name))
	require.NotEmpty(t, code)
	// This registration does not request generated parameters. Exercise the
	// same cache/fallback helper with its source to cover a deliberate cache miss.
	_, hit := builtinmetadata.Lookup("plugin:mitm:"+name, code)
	require.False(t, hit)
	prog, err := static_analyzer.SSAParse(code, "mitm")
	require.NoError(t, err)
	params, envKeys, err := information.GenerateParameterFromProgram(prog)
	require.NoError(t, err)
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.YakScript{}).Error)
	stored := &schema.YakScript{ScriptName: name, Content: code, Type: "mitm", IsCorePlugin: true, Params: "outdated"}
	require.NoError(t, db.Create(stored).Error)
	id := stored.ID
	require.NoError(t, overwriteYakPlugin(db, name, &schema.YakScript{ScriptName: name, Content: code, Type: "mitm"}, true))
	require.NoError(t, db.First(stored, id).Error)
	require.Equal(t, id, stored.ID)
	require.Equal(t, params, stored.Params)
	require.Equal(t, envKeys, stored.PluginEnvKey)
	require.Error(t, overwriteYakPlugin(db, "missing-builtin", &schema.YakScript{}, true))
	require.NoError(t, db.Close())
	require.Error(t, overwriteYakPlugin(db, name, &schema.YakScript{}, true))
}
