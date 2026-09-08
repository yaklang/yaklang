package yakgrpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp"
	mcpmodel "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestScreenshotMCPDuplexRoundTrip(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := client.DuplexConnection(ctx)
	require.NoError(t, err)

	// A following frame is a barrier proving the subscription has been processed.
	ready := make(chan struct{}, 1)
	barrier := "screenshot-test-" + uuid.NewString()
	yakit.YakitDuplexConnectionServer.RegisterHandler(barrier, func(context.Context, *ypb.DuplexConnectionRequest) error {
		ready <- struct{}{}
		return nil
	})
	defer yakit.YakitDuplexConnectionServer.UnRegisterHandler(barrier)
	require.NoError(t, stream.Send(&ypb.DuplexConnectionRequest{MessageType: yakit.ScreenshotSubscribe}))
	require.NoError(t, stream.Send(&ypb.DuplexConnectionRequest{MessageType: barrier}))
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	var buffer bytes.Buffer
	require.NoError(t, png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 800, 600))))
	encoded := base64.StdEncoding.EncodeToString(buffer.Bytes())
	frontendDone := make(chan error, 1)
	go func() {
		for captures := 0; captures < 2; {
			message, err := stream.Recv()
			if err != nil {
				frontendDone <- err
				return
			}
			if message.GetMessageType() != yakit.ScreenshotRequest {
				continue
			}
			var reply yakit.YakitScreenshot
			if err := json.Unmarshal(message.Data, &reply); err != nil {
				frontendDone <- err
				return
			}
			reply.Data, reply.Width, reply.Height = encoded, 800, 600
			reply.CapturedAt = "2026-09-08 12:34:56 +08:00"
			data, _ := json.Marshal(reply)
			if err := stream.Send(&ypb.DuplexConnectionRequest{MessageType: yakit.ScreenshotResponse, Data: data}); err != nil {
				frontendDone <- err
				return
			}
			captures++
		}
		frontendDone <- nil
	}()
	result, err := mcp.CallBuiltinTool(&mcp.MCPServer{}, ctx, "screenshot", nil)
	require.NoError(t, err)
	content, ok := mcpmodel.AsImageContent(result.Content[1])
	require.True(t, ok)
	require.Equal(t, "image/png", content.MIMEType)
	require.Equal(t, encoded, content.Data)

	path := filepath.Join(t.TempDir(), "httpflow.png")
	result, err = mcp.CallBuiltinTool(&mcp.MCPServer{}, ctx, "screenshot", map[string]any{"savePath": path})
	require.NoError(t, err)
	require.NoError(t, <-frontendDone)
	require.Len(t, result.Content, 1)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].(mcpmodel.TextContent).Text), &metadata))
	require.Equal(t, "file", metadata["delivery"])
	require.Equal(t, path, metadata["path"])
	require.NotContains(t, metadata, "data")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, encoded, base64.StdEncoding.EncodeToString(data))
}
