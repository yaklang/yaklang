package aiforge

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
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
