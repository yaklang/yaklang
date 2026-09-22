package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

func pointer[T any](v T) *T { return &v }

type fakeUploadStream struct {
	grpc.ServerStream
	SendMessages []*ypb.DownloadProgress
	sendErr      error
}

func (f *fakeUploadStream) Send(resp *ypb.DownloadProgress) error {
	f.SendMessages = append(f.SendMessages, resp)
	return f.sendErr
}
func (f *fakeUploadStream) Context() context.Context { return context.Background() }

type fakeDownloadStream = fakeUploadStream

func TestUploadPayloadToOnline(t *testing.T) {
	for _, scenario := range []string{"file", "inline", "missing file", "remote error", "empty selection"} {
		t.Run(scenario, func(t *testing.T) {
			db := newOnlineTestDB(t, &schema.Payload{})
			filename := filepath.Join(t.TempDir(), "payload.txt")
			require.NoError(t, os.WriteFile(filename, []byte("mock file content"), 0600))
			content := filename
			isFile := scenario != "inline"
			if !isFile {
				content = "inline content"
			}
			if scenario == "missing file" {
				content += ".missing"
			}
			p := &schema.Payload{Group: "test-group", Folder: pointer("test-folder"), Content: &content, IsFile: &isFile}
			require.NoError(t, db.Create(p).Error)
			// A distractor detects accidentally dropped SQL predicates.
			other := &schema.Payload{Group: "other", Folder: pointer("test-folder"), Content: pointer("other"), IsFile: pointer(false)}
			require.NoError(t, db.Create(other).Error)
			calls := 0
			remote := &stubOnlineService{uploadPayload: func(ctx context.Context, token string, data, file []byte) error {
				calls++
				require.Equal(t, "fake-token", token)
				var got schema.Payload
				require.NoError(t, json.Unmarshal(data, &got))
				require.Equal(t, "test-group", got.Group)
				if isFile {
					require.Equal(t, "mock file content", string(file))
				} else {
					require.Empty(t, file)
					require.Equal(t, content, *got.Content)
				}
				if scenario == "remote error" {
					return errors.New("remote unavailable")
				}
				return nil
			}}
			server := &Server{profileDatabase: db, onlineClient: remote}
			req := &ypb.UploadPayloadToOnlineRequest{Token: "fake-token", Group: "test-group", Folder: "test-folder"}
			if scenario == "empty selection" {
				req.Folder = "absent"
			}
			stream := &fakeUploadStream{}
			require.NoError(t, server.UploadPayloadToOnline(req, stream))
			require.NotEmpty(t, stream.SendMessages)
			final := stream.SendMessages[len(stream.SendMessages)-1]
			require.Equal(t, 1.0, final.Progress)
			switch scenario {
			case "missing file":
				require.Zero(t, calls)
				require.Equal(t, "error", final.MessageType)
			case "remote error":
				require.Equal(t, 1, calls)
				require.Equal(t, "error", final.MessageType)
			case "empty selection":
				require.Zero(t, calls)
				require.Equal(t, "warning", final.MessageType)
			default:
				require.Equal(t, 1, calls)
				require.Equal(t, "success", final.MessageType)
			}
		})
	}
}

func TestDownloadPayload(t *testing.T) {
	db := newOnlineTestDB(t, &schema.Payload{})
	remote := &stubOnlineService{downloadPayload: func(ctx context.Context, token, group, folder string) *yaklib.OnlineDownloadPayloadStream {
		require.Equal(t, "fake-token", token)
		require.Equal(t, "test-group", group)
		require.Equal(t, "test-folder", folder)
		ch := make(chan *yaklib.OnlinePayloadItem, 1)
		ch <- &yaklib.OnlinePayloadItem{PayloadData: &yaklib.OnlinePayload{Group: group, Folder: folder, Content: "test content", Hash: "mockhash"}, Total: 1}
		close(ch)
		return &yaklib.OnlineDownloadPayloadStream{Chan: ch, Total: 1}
	}}
	server := &Server{profileDatabase: db, onlineClient: remote}
	stream := &fakeDownloadStream{}
	req := &ypb.DownloadPayloadRequest{Token: "fake-token", Group: "test-group", Folder: "test-folder"}
	require.NoError(t, server.DownloadPayload(req, stream))
	require.Len(t, stream.SendMessages, 3)
	require.Equal(t, "success", stream.SendMessages[2].MessageType)
	var saved schema.Payload
	require.NoError(t, db.First(&saved).Error)
	require.Equal(t, req.Group, saved.Group)
	require.Equal(t, req.Folder, *saved.Folder)
	require.Contains(t, *saved.Content, "test content")
	// A stream failure must be visible to the caller.
	stream.sendErr = errors.New("stream closed")
	require.ErrorContains(t, server.DownloadPayload(req, stream), "stream closed")
	remote.downloadPayload = func(context.Context, string, string, string) *yaklib.OnlineDownloadPayloadStream { return nil }
	require.ErrorContains(t, server.DownloadPayload(req, &fakeDownloadStream{}), "initialization failed")
}
