package yakurl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newFileSearchParams(path string, query ...*ypb.KVPair) *ypb.RequestYakURLParams {
	return &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema: "file",
			Path:   path,
			Query: append([]*ypb.KVPair{
				{Key: "op", Value: "search"},
			}, query...),
		},
	}
}

// searchPayloadOf decodes the JSON payload published for one matched file.
func searchPayloadOf(t *testing.T, resource *ypb.YakURLResource) *fsSearchResult {
	t.Helper()
	for _, item := range resource.GetExtra() {
		if item.GetKey() != searchResultExtraKey {
			continue
		}
		var result fsSearchResult
		require.NoError(t, json.Unmarshal([]byte(item.GetValue()), &result))
		return &result
	}
	t.Fatalf("resource[%s] carries no %s extra", resource.GetPath(), searchResultExtraKey)
	return nil
}

func TestFileSystemActionSearchGlobalAggregatesByFile(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("top.txt", "needle one\nno match\nneedle two\n")
	vfs.AddFile("nested/child.txt", "before\nneedle child\nafter")
	vfs.AddFile("nested/other.txt", "nothing here")
	action := &fileSystemAction{fs: vfs}

	response, err := action.Get(newFileSearchParams(".",
		&ypb.KVPair{Key: "keyword", Value: "needle"},
		&ypb.KVPair{Key: "global", Value: "true"},
	))
	require.NoError(t, err)
	require.Equal(t, int64(2), response.GetTotal())
	require.Len(t, response.GetResources(), 2)

	require.Equal(t, "top.txt", response.GetResources()[0].GetPath())
	top := searchPayloadOf(t, response.GetResources()[0])
	require.Equal(t, 2, top.MatchCount)
	require.Equal(t, []fsSearchLine{
		{Line: 1, Text: "needle one", Matches: []fsSearchMatch{{StartColumn: 1, EndColumn: 7}}},
		{Line: 3, Text: "needle two", Matches: []fsSearchMatch{{StartColumn: 1, EndColumn: 7}}},
	}, top.Results)

	require.Equal(t, "nested/child.txt", response.GetResources()[1].GetPath())
	child := searchPayloadOf(t, response.GetResources()[1])
	require.Equal(t, 1, child.MatchCount)
	require.Equal(t, []fsSearchLine{
		{Line: 2, Text: "needle child", Matches: []fsSearchMatch{{StartColumn: 1, EndColumn: 7}}},
	}, child.Results)
}

func TestFileSystemActionSearchWithoutGlobalDoesNotRecurse(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("top.txt", "needle")
	vfs.AddFile("nested/child.txt", "needle")
	action := &fileSystemAction{fs: vfs}

	response, err := action.Get(newFileSearchParams(".",
		&ypb.KVPair{Key: "keyword", Value: "needle"},
	))
	require.NoError(t, err)
	require.Equal(t, int64(1), response.GetTotal())
	require.Equal(t, "top.txt", response.GetResources()[0].GetPath())
}

// A line matching more than once must stay one result entry carrying every hit,
// because the frontend highlights and steps through hits, not lines.
func TestFileSystemActionSearchReportsEveryHitOnOneLine(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("top.txt", "needle one needle\nno match\n")
	action := &fileSystemAction{fs: vfs}

	response, err := action.Get(newFileSearchParams("top.txt",
		&ypb.KVPair{Key: "keyword", Value: "needle"},
	))
	require.NoError(t, err)
	require.Equal(t, int64(1), response.GetTotal())

	payload := searchPayloadOf(t, response.GetResources()[0])
	require.Equal(t, 2, payload.MatchCount)
	require.Equal(t, []fsSearchLine{
		{
			Line: 1,
			Text: "needle one needle",
			Matches: []fsSearchMatch{
				{StartColumn: 1, EndColumn: 7},
				{StartColumn: 12, EndColumn: 18},
			},
		},
	}, payload.Results)
}

// Columns are UTF-16 code units of the published text: 中文 is two units and an
// emoji is also two, so both keep the match at column 3.
func TestFileSystemActionSearchReportsUTF16Columns(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("wide.txt", "中文needle\n🚀needle\n")
	action := &fileSystemAction{fs: vfs}

	response, err := action.Get(newFileSearchParams("wide.txt",
		&ypb.KVPair{Key: "keyword", Value: "needle"},
	))
	require.NoError(t, err)

	payload := searchPayloadOf(t, response.GetResources()[0])
	require.Equal(t, 2, payload.MatchCount)
	require.Equal(t, []fsSearchLine{
		{Line: 1, Text: "中文needle", Matches: []fsSearchMatch{{StartColumn: 3, EndColumn: 9}}},
		{Line: 2, Text: "🚀needle", Matches: []fsSearchMatch{{StartColumn: 3, EndColumn: 9}}},
	}, payload.Results)
}

// Control bytes are escaped for transport, which makes the published text
// longer than the raw line. Columns have to follow the published text.
func TestFileSystemActionSearchKeepsColumnsAfterEscaping(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("colored.log", "\x1b[31mneedle\x1b[0m\n")
	action := &fileSystemAction{fs: vfs}

	response, err := action.Get(newFileSearchParams("colored.log",
		&ypb.KVPair{Key: "keyword", Value: "needle"},
	))
	require.NoError(t, err)

	payload := searchPayloadOf(t, response.GetResources()[0])
	require.Equal(t, []fsSearchLine{
		{
			Line:    1,
			Text:    `\x1b[31mneedle\x1b[0m`,
			Matches: []fsSearchMatch{{StartColumn: 9, EndColumn: 15}},
		},
	}, payload.Results)
}

func TestFileSystemActionSearchTrimsCRLF(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("dos.txt", "needle one\r\nneedle two\r\n")
	action := &fileSystemAction{fs: vfs}

	response, err := action.Get(newFileSearchParams("dos.txt",
		&ypb.KVPair{Key: "keyword", Value: "needle"},
	))
	require.NoError(t, err)

	payload := searchPayloadOf(t, response.GetResources()[0])
	require.Equal(t, []fsSearchLine{
		{Line: 1, Text: "needle one", Matches: []fsSearchMatch{{StartColumn: 1, EndColumn: 7}}},
		{Line: 2, Text: "needle two", Matches: []fsSearchMatch{{StartColumn: 1, EndColumn: 7}}},
	}, payload.Results)
}

func TestFileSystemActionSearchRegexpAndIgnoreCase(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("sample.txt", "prefix yak-123 suffix\r\nYAK-456\r\nYak-abc")
	action := &fileSystemAction{fs: vfs}

	response, err := action.Get(newFileSearchParams("sample.txt",
		&ypb.KVPair{Key: "keyword", Value: `(?<=yak-)\d+`},
		&ypb.KVPair{Key: "regex", Value: "true"},
		&ypb.KVPair{Key: "ignoreCase", Value: "true"},
	))
	require.NoError(t, err)
	require.Equal(t, int64(1), response.GetTotal())

	payload := searchPayloadOf(t, response.GetResources()[0])
	require.Equal(t, []fsSearchLine{
		{
			Line:    1,
			Text:    "prefix yak-123 suffix",
			Matches: []fsSearchMatch{{StartColumn: 12, EndColumn: 15}},
		},
		{
			Line:    2,
			Text:    "YAK-456",
			Matches: []fsSearchMatch{{StartColumn: 5, EndColumn: 8}},
		},
	}, payload.Results)
}

func TestFileSystemActionSearchLiteralByDefault(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("sample.txt", "a.c\nabc")
	action := &fileSystemAction{fs: vfs}

	response, err := action.Get(newFileSearchParams("sample.txt",
		&ypb.KVPair{Key: "keyword", Value: "a.c"},
	))
	require.NoError(t, err)

	payload := searchPayloadOf(t, response.GetResources()[0])
	require.Equal(t, []fsSearchLine{
		{Line: 1, Text: "a.c", Matches: []fsSearchMatch{{StartColumn: 1, EndColumn: 4}}},
	}, payload.Results)
}

func TestFileSystemActionSearchRejectsInvalidRegexp(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("sample.txt", "anything")
	action := &fileSystemAction{fs: vfs}

	_, err := action.Get(newFileSearchParams("sample.txt",
		&ypb.KVPair{Key: "keyword", Value: "["},
		&ypb.KVPair{Key: "regex", Value: "true"},
	))
	require.ErrorContains(t, err, "invalid search regular expression")
}
