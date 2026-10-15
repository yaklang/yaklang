package aiforge

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/builtinmetadata"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// buildInForgeYakEmbedRelPaths 与 getBuildInForgeYakScript / getBuildInForgeConfig 的目录约定一致（平铺、子目录、下划线前缀）。
func buildInForgeYakEmbedRelPaths(name string) []string {
	return []string{
		path.Join("buildinforge", name+".yak"),
		path.Join("buildinforge", name, name+".yak"),
		path.Join("buildinforge", name, "_"+name+".yak"),
	}
}

func readEmbeddedYakBytesForBuildInForge(name string) []byte {
	InitEmbedFS()
	for _, p := range buildInForgeYakEmbedRelPaths(name) {
		b, err := buildInForgeFS.ReadFile(p)
		if err == nil && len(b) > 0 {
			return b
		}
	}
	return nil
}

func TestBuildInForgeFromFS_Loads(t *testing.T) {
	InitEmbedFS()
	_ = syncBuildInForgeInternal()
	names := RegisteredBuildInForgeNames()
	require.NotEmpty(t, names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			forge, err := getBuildInForgeFromFS(name)
			require.NoError(t, err, "getBuildInForgeFromFS(%q)", name)
			require.NotNil(t, forge)
			require.NotEmpty(t, forge.ForgeName)
		})
	}
}

func TestBuildInForgeEmbeddedYak_SSAParse(t *testing.T) {
	InitEmbedFS()
	_ = syncBuildInForgeInternal()
	names := RegisteredBuildInForgeNames()
	require.NotEmpty(t, names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			src := readEmbeddedYakBytesForBuildInForge(name)
			if len(src) == 0 {
				t.Skip("no embedded .yak for this forge")
			}
			_, err := static_analyzer.SSAParse(string(src), "yak")
			require.NoError(t, err, "SSAParse buildin forge %q yak source", name)
		})
	}
}

func TestInitEmbedFS(t *testing.T) {
	InitEmbedFS()
	hash, err := BuildInForgeHash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestGetBuildInForgeFromFS_DefaultAuthor(t *testing.T) {
	InitEmbedFS()

	forge, err := getBuildInForgeFromFS("web_log_monitor")
	assert.NoError(t, err)
	assert.NotNil(t, forge)
	assert.Equal(t, schema.AIResourceAuthorBuiltin, forge.Author)
	assert.Equal(t, true, forge.IsBuiltin)
}

func TestBuiltinForgeMetadataMatchesDynamicAnalysis(t *testing.T) {
	for _, name := range []string{"web_log_monitor", "flow_report", "ssa_vulnerability_analyzer"} {
		t.Run(name, func(t *testing.T) {
			code := readEmbeddedYakBytesForBuildInForge(name)
			require.NotEmpty(t, code)
			_, hit := builtinmetadata.Lookup("forge:"+name, string(code))
			require.Equal(t, builtinmetadata.Enabled, hit)
			cached, err := buildAIForgeFromYakCode(name, code)
			require.NoError(t, err)
			// A different name cannot hit the builtin catalog. All derived
			// information must agree with the full SSA extraction path.
			dynamic, err := buildAIForgeFromYakCode("user-"+name, code)
			require.NoError(t, err)
			dynamic.ForgeName = name
			require.Equal(t, dynamic, cached)
		})
	}
}

func TestBuiltinForgeSyncPreservesUserPreferences(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AIForge{}).Error)
	forge := &schema.AIForge{ForgeName: "web_log_monitor", UserPersistentData: "user preferences", ForgeContent: "old source"}
	require.NoError(t, db.Create(forge).Error)
	builtin, err := getBuildInForgeFromFS(forge.ForgeName)
	require.NoError(t, err)
	require.NoError(t, saveBuiltinForge(db, builtin))
	stored, err := yakit.GetAIForgeByName(db, forge.ForgeName)
	require.NoError(t, err)
	require.Equal(t, forge.ID, stored.ID)
	require.Equal(t, forge.UserPersistentData, stored.UserPersistentData)
	require.NotEqual(t, "old source", stored.ForgeContent)
}
