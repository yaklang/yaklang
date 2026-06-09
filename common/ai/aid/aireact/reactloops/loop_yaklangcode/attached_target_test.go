package loop_yaklangcode

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestYaklangFilePathFromAttachedSelected(t *testing.T) {
	payload := `{"path":"/tmp/foo.yak","startLine":1,"endLine":2,"language":"yak","content":"x=1"}`
	path := yaklangFilePathFromAttached([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, payload),
	})
	require.Equal(t, filepath.Clean("/tmp/foo.yak"), path)
}

func TestYaklangFilePathFromAttachedFilePath(t *testing.T) {
	path := yaklangFilePathFromAttached([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyEditorFile, "/tmp/bar.yak"),
	})
	require.Equal(t, filepath.Clean("/tmp/bar.yak"), path)
}

func TestYaklangFilePathFromAttachedPrefersSelected(t *testing.T) {
	payload := `{"path":"/tmp/selected.yak","startLine":1,"endLine":2,"language":"yak","content":"x=1"}`
	path := yaklangFilePathFromAttached([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, payload),
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyEditorFile, "/tmp/file.yak"),
	})
	require.Equal(t, filepath.Clean("/tmp/selected.yak"), path)
}
