package thirdparty_bin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsYaklangCodeAIKBDownload(t *testing.T) {
	assert.True(t, IsYaklangCodeAIKBDownload("Yaklang AI KnowledgeBase", "yaklang-aikb.rag.gz"))
	assert.True(t, IsYaklangCodeAIKBDownload("", "yaklang-aikb.rag.gz"))
	assert.False(t, IsYaklangCodeAIKBDownload("CWE Knowledge Base", "cwe.rag.gz"))
}

func TestInstallPathCandidates_RAGGzipAlias(t *testing.T) {
	tmp := t.TempDir()
	ragPath := filepath.Join(tmp, "yaklang-aikb.rag")
	gzPath := ragPath + ".gz"
	require.NoError(t, os.WriteFile(gzPath, []byte("YAKRAG"), 0o644))

	bi := NewInstaller(tmp, t.TempDir()).(*BaseInstaller)
	desc := &BinaryDescriptor{
		Name: YaklangCodeAIKBRagName,
		DownloadInfoMap: map[string]*DownloadInfo{
			"*": {BinPath: "yaklang-aikb.rag"},
		},
	}

	got := bi.GetInstallPath(desc, nil)
	assert.Equal(t, gzPath, got)
}
