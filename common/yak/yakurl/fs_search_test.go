package yakurl

import (
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

func searchMatchMap(resource *ypb.YakURLResource) map[string]string {
	matches := make(map[string]string)
	for _, item := range resource.GetExtra() {
		if item.GetKey() == "Directory-Name" {
			continue
		}
		matches[item.GetKey()] = item.GetValue()
	}
	return matches
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
	require.Equal(t, map[string]string{
		"1": "needle one",
		"3": "needle two",
	}, searchMatchMap(response.GetResources()[0]))

	require.Equal(t, "nested/child.txt", response.GetResources()[1].GetPath())
	require.Equal(t, map[string]string{
		"2": "needle child",
	}, searchMatchMap(response.GetResources()[1]))
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
	require.Equal(t, map[string]string{
		"1": "prefix yak-123 suffix",
		"2": "YAK-456",
	}, searchMatchMap(response.GetResources()[0]))
}

func TestFileSystemActionSearchLiteralByDefault(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("sample.txt", "a.c\nabc")
	action := &fileSystemAction{fs: vfs}

	response, err := action.Get(newFileSearchParams("sample.txt",
		&ypb.KVPair{Key: "keyword", Value: "a.c"},
	))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"1": "a.c"}, searchMatchMap(response.GetResources()[0]))
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
