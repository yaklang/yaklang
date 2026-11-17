package aicommon

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestAttachedFileContentIsLiteral(t *testing.T) {
	path := t.TempDir() + "/existing.txt"
	require.NoError(t, os.WriteFile(path, []byte("DO_NOT_READ_THIS"), 0600))
	for _, content := range []string{path, "  literal text\n", ""} {
		resource, err := ParseAttachedResourceData(NewAttachedResource("file", "file_content", content))
		require.NoError(t, err)
		require.IsType(t, &AttachedFileContentResourceData{}, resource)
		fileResource := resource.(*AttachedFileContentResourceData)
		require.NoError(t, resource.BindLoopData(nil))
		require.NotEmpty(t, fileResource.savedPath)
		t.Cleanup(func() { _ = os.Remove(fileResource.savedPath) })
		rendered := resource.ToAttachData(nil)
		require.Contains(t, rendered, "--- File Content ---\n"+content+"\n--- End")
		require.Contains(t, rendered, "Full content saved to file: "+fileResource.savedPath)
		require.NotContains(t, rendered, "DO_NOT_READ_THIS")
		require.NotContains(t, rendered, "failed to stat")
	}
}

func TestAttachedFileContentReadBack(t *testing.T) {
	content := " \n " + strings.Repeat("中文日志", 18000) + "MIDDLE_EVIDENCE" + strings.Repeat("entry\n", 18000) + "TAIL_EVIDENCE\n "
	resource, err := ParseAttachedResourceData(NewAttachedResource("file", "file_content", content))
	require.NoError(t, err)
	require.NoError(t, resource.BindLoopData(nil))
	rendered := resource.ToAttachData(nil)
	require.True(t, utf8.ValidString(rendered))
	require.Less(t, len(rendered), 10*1024)
	require.NotContains(t, rendered, "MIDDLE_EVIDENCE")
	_, after, found := strings.Cut(rendered, "Full content saved to file: ")
	require.True(t, found)
	path, _, _ := strings.Cut(after, "\n")
	t.Cleanup(func() { _ = os.Remove(path) })
	full, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(full))
	for i := 0; i < 3; i++ {
		require.Equal(t, rendered, resource.ToAttachData(nil))
	}
}
