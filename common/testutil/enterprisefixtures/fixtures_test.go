package enterprisefixtures

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnterpriseFixtureArchiveExtractsAllCases(t *testing.T) {
	repo, err := repositoryRoot()
	require.NoError(t, err)
	archive := filepath.Join(repo, filepath.FromSlash(archiveRelativePath))
	destination := t.TempDir()
	require.NoError(t, ExtractArchive(archive, destination))

	for _, relative := range requiredExpandedFiles {
		info, err := os.Stat(filepath.Join(destination, filepath.FromSlash(relative)))
		require.NoError(t, err, relative)
		require.True(t, info.Mode().IsRegular(), relative)
	}
	for _, relative := range []string{
		"dns-tunnel-c2/TASKS.md",
		"https-keylog-dridex/capture.keys",
		"lateral-movement-psexec/manifest.json",
	} {
		info, err := os.Stat(filepath.Join(destination, filepath.FromSlash(relative)))
		require.NoError(t, err, relative)
		require.True(t, info.Mode().IsRegular(), relative)
	}
}
