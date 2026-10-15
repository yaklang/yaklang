//go:build !irify_exclude

package sfbuildin

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/builtinmetadata"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestBuiltinSyntaxFlowMetadataMatchesParser(t *testing.T) {
	require.NoError(t, filesys.Recursive(".", filesys.WithFileSystem(ruleFSWithHash), filesys.WithFileStat(func(name string, info fs.FileInfo) error {
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}
		content, err := ruleFSWithHash.ReadFile(name)
		require.NoError(t, err)
		relative := strings.TrimPrefix(strings.TrimPrefix(path.Clean(name), "./"), "buildin/")
		_, hit := builtinmetadata.Lookup("sf:"+relative, string(content))
		require.Equal(t, builtinmetadata.Enabled, hit, "cache activation for %s", name)
		// Ordinary builds keep the original parser; gzip builds must be identical.
		if !builtinmetadata.Enabled {
			return nil
		}
		cached, err := parseBuiltinRule(name, string(content), true)
		require.NoError(t, err)
		dynamic, err := sfdb.CheckSyntaxFlowRuleContent(string(content))
		require.NoError(t, err)
		var wantOps, gotOps sfvm.OpCodes
		require.NoError(t, json.Unmarshal([]byte(dynamic.OpCodes), &wantOps))
		require.NoError(t, json.Unmarshal([]byte(cached.OpCodes), &gotOps))
		require.Equal(t, wantOps, gotOps, "cached instructions differ for %s", name)
		dynamic.OpCodes, cached.OpCodes = "", ""
		require.Equal(t, dynamic, cached, "cached rule differs for %s", name)
		return nil
	})))
}

func TestBuiltinSyntaxFlowChangedSourceFallsBack(t *testing.T) {
	content := `desc(title:"changed source");* as $new;alert $new;`
	_, hit := builtinmetadata.Lookup("sf:java/cwe-89-sql-injection.sf", content)
	require.False(t, hit)
	got, err := parseBuiltinRule("buildin/java/cwe-89-sql-injection.sf", content, true)
	require.NoError(t, err)
	want, err := sfdb.CheckSyntaxFlowRuleContent(content)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestBuiltinRuleSyncMarkerIsStableInDevelopment(t *testing.T) {
	oldVersion := consts.GetYakVersion()
	consts.SetYakVersion("dev")
	defer consts.SetYakVersion(oldVersion)
	old := yakit.Get(consts.EmbedSfBuildInRuleKey)
	defer yakit.Set(consts.EmbedSfBuildInRuleKey, old)
	yakit.Set(consts.EmbedSfBuildInRuleKey, "")
	require.True(t, NeedSyncEmbedRule())
	DoneEmbedRule()
	require.False(t, NeedSyncEmbedRule(), "a successful sync must stay current in both build modes")
}
