package loop_ssa_api_discovery

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func testReadFileActionJSON(fields string) *aicommon.Action {
	maker := aicommon.NewActionMaker("read_file")
	raw := `{"@action":"read_file",` + fields + `}`
	return maker.ReadFromReader(context.Background(), strings.NewReader(raw))
}

func TestReadFileParamsForBuiltin_PrefersFileOverPath(t *testing.T) {
	action := testReadFileActionJSON(`"file":"/abs/a.java","path":"/should/ignore"`)
	params := readFileParamsForBuiltin(action)
	require.Equal(t, "/abs/a.java", params.GetString("file"))
	_, hasPath := params["path"]
	require.False(t, hasPath)
}

func TestReadFileParamsForBuiltin_MapsLegacyPath(t *testing.T) {
	action := testReadFileActionJSON(`"path":"/legacy/path.java"`)
	params := readFileParamsForBuiltin(action)
	require.Equal(t, "/legacy/path.java", params.GetString("file"))
	_, hasPath := params["path"]
	require.False(t, hasPath)
}

func TestNormalizeReadFilePath(t *testing.T) {
	require.Empty(t, normalizeReadFilePath(testReadFileActionJSON("")))
	action := testReadFileActionJSON(`"path":"/p"`)
	require.Equal(t, "/p", normalizeReadFilePath(action))
	action = testReadFileActionJSON(`"path":"/p","file":"/f"`)
	require.Equal(t, "/f", normalizeReadFilePath(action))
}

func TestReadFileParamsForBuiltin_PreservesOffsetLines(t *testing.T) {
	action := testReadFileActionJSON(`"file":"/f.java","offset":10,"lines":50`)
	params := readFileParamsForBuiltin(action)
	require.Equal(t, "/f.java", params.GetString("file"))
	require.Equal(t, 10, params.GetInteger("offset"))
	require.Equal(t, 50, params.GetInteger("lines"))
}
