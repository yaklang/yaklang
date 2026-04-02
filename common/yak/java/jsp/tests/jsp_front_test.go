package tests

import (
	"embed"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/jsp"
)

const savedJSPFrontFixtureMaxParseDuration = 30 * time.Second

//go:embed all:code
var jspFs embed.FS

func validateJSPFrontFixture(t *testing.T, filePath string, src string) {
	t.Helper()

	start := time.Now()
	_, err := jsp.Front(src)
	parseDur := time.Since(start)
	require.NoError(t, err, "error in file: %s", filePath)
	require.LessOrEqual(t, parseDur, savedJSPFrontFixtureMaxParseDuration, "parse took too long for %s", filePath)
}

func TestAllJSPFrontFixtures(t *testing.T) {
	found := false
	err := fs.WalkDir(jspFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filePath)
		if !strings.HasSuffix(ext, ".jsp") && !strings.HasSuffix(ext, ".jspx") {
			return nil
		}

		raw, err := jspFs.ReadFile(filePath)
		require.NoError(t, err)
		t.Run(filePath, func(t *testing.T) {
			validateJSPFrontFixture(t, filePath, string(raw))
		})
		found = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, found, "no embed jsp fixtures found")
}
