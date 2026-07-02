package loop_fast_context

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func readTestAction(t *testing.T, json string) *aicommon.Action {
	t.Helper()
	ctx := context.Background()
	action := (&aicommon.ActionMaker{}).ReadFromReader(ctx, strings.NewReader(json))
	require.NotNil(t, action)
	action.WaitStream(ctx)
	return action
}

func TestParseGrepBatchSearches_structArray(t *testing.T) {
	action := readTestAction(t, `{
		"@action": "grep_files_batch",
		"searches": [
			{"path": "/tmp/proj", "pattern": "gob\\.Decode", "max": 50},
			{"id": "unmarshal", "path": "/tmp/proj", "pattern": "json\\.Unmarshal"}
		]
	}`)

	searches, err := parseGrepBatchSearches(action)
	require.NoError(t, err)
	require.Len(t, searches, 2)
	require.Equal(t, "search_1", searches[0].ID)
	require.Equal(t, "unmarshal", searches[1].ID)
	require.Equal(t, "files_with_matches", searches[0].Params.GetString("output-mode"))
	require.Equal(t, int64(50), searches[0].Params.GetInt("limit"))
	require.Equal(t, int64(grepFilesWithMatchesLimit), searches[1].Params.GetInt("limit"))
}

func TestParseGrepBatchSearches_jsonString(t *testing.T) {
	action := readTestAction(t, `{
		"@action": "grep_files_batch",
		"searches": "[{\"path\":\"/tmp/proj\",\"pattern\":\"exec\\\\(\"},{\"path\":\"/tmp/proj\",\"pattern\":\"system\\\\(\"}]"
	}`)

	searches, err := parseGrepBatchSearches(action)
	require.NoError(t, err)
	require.Len(t, searches, 2)
}

func TestParseGrepBatchSearches_rejectsEmpty(t *testing.T) {
	action := readTestAction(t, `{"@action":"grep_files_batch","searches":[]}`)

	_, err := parseGrepBatchSearches(action)
	require.Error(t, err)
}

func TestParseGrepBatchSearches_requiresPathAndPattern(t *testing.T) {
	action := readTestAction(t, `{
		"@action": "grep_files_batch",
		"searches": [{"path": "/tmp/proj"}]
	}`)

	_, err := parseGrepBatchSearches(action)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pattern")
}

func TestNormalizeGrepSearchParams_forcesFilesWithMatches(t *testing.T) {
	params := normalizeGrepSearchParams(aitool.InvokeParams{
		"path":        "/tmp",
		"pattern":     "foo",
		"output-mode": "content",
		"include-ext": "go",
	})
	require.Equal(t, "files_with_matches", params.GetString("output-mode"))
	require.Equal(t, int64(grepFilesWithMatchesLimit), params.GetInt("limit"))
}
