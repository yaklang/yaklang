package mcp

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func TestScreenshotResourceCannotReadUnregisteredOrChangedFiles(t *testing.T) {
	ctx := context.Background()
	fixture := screenshotPNGFixture(t, false)
	raw, err := base64.StdEncoding.DecodeString(fixture.Data)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "capture.png")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	img, err := aimedia.FromFile(ctx, path)
	require.NoError(t, err)
	s := &MCPServer{}
	request := mcp.ReadResourceRequest{}
	request.Params.URI = "yakit-screenshot://capture"
	_, err = s.readScreenshotResource(ctx, request)
	require.ErrorContains(t, err, "not available")
	s.screenshotResources.Store(request.Params.URI, screenshotResource{Image: img, Expires: time.Now().Add(time.Minute)})
	contents, err := s.readScreenshotResource(ctx, request)
	require.NoError(t, err)
	require.Equal(t, fixture.Data, contents[0].(mcp.BlobResourceContents).Blob)
	require.NoError(t, os.WriteFile(path, []byte("changed"), 0o600))
	_, err = s.readScreenshotResource(ctx, request)
	require.ErrorContains(t, err, "does not match")
	s.screenshotResources.Store(request.Params.URI, screenshotResource{Image: img, Expires: time.Now().Add(-time.Minute)})
	_, err = s.readScreenshotResource(ctx, request)
	require.ErrorContains(t, err, "expired")
}
