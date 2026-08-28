package yakgrpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// History 中的 File/PDF GUI 会用这个既有 RPC 保存编辑工作副本。
// 这里用包含 NUL/高位字节的数据约束它必须按 bytes 原样落盘，而不是按文本转码；
// cleanup 无论断言成功或失败都会执行，避免污染本地和 CI 的 Yakit 临时目录。
func TestSaveTextToTemporalFilePreservesBinaryBytes(t *testing.T) {
	want := []byte{0x25, 0x50, 0x44, 0x46, 0x00, 0xff, 0x80, 0x0a}
	rsp, err := new(Server).SaveTextToTemporalFile(context.Background(), &ypb.SaveTextToTemporalFileRequest{
		Text: want,
	})
	require.NoError(t, err)
	require.NotEmpty(t, rsp.GetFileName())
	t.Cleanup(func() {
		_ = os.Remove(rsp.GetFileName())
	})

	require.FileExists(t, rsp.GetFileName())
	rel, err := filepath.Rel(consts.GetDefaultYakitBaseTempDir(), rsp.GetFileName())
	require.NoError(t, err)
	require.False(t, rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)))
	got, err := os.ReadFile(rsp.GetFileName())
	require.NoError(t, err)
	require.True(t, bytes.Equal(want, got), "binary working copy must be byte-for-byte identical")
}

// History/WebFuzzer replacement files originate on the GUI machine but file
// Fuzztag paths belong to the engine filesystem. This contract verifies that
// chunk boundaries do not affect bytes and that the returned path is engine-local.
func TestReceiveTemporaryFileUploadPreservesChunkedBinaryBytes(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	chunks := [][]byte{{0x25, 0x50}, nil, {0x44, 0x46, 0x00, 0xff, 0x80}}
	var response *ypb.UploadToTemporaryFileResponse

	err := receiveTemporaryFileUpload(func() (*ypb.UploadToTemporaryFileRequest, error) {
		if len(chunks) == 0 {
			return nil, io.EOF
		}
		chunk := chunks[0]
		chunks = chunks[1:]
		return &ypb.UploadToTemporaryFileRequest{Data: chunk}, nil
	}, func(rsp *ypb.UploadToTemporaryFileResponse) error {
		response = rsp
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	t.Cleanup(func() { _ = os.Remove(response.GetFileName()) })
	require.Equal(t, int64(7), response.GetSize())
	require.FileExists(t, response.GetFileName())
	got, err := os.ReadFile(response.GetFileName())
	require.NoError(t, err)
	require.Equal(t, []byte{0x25, 0x50, 0x44, 0x46, 0x00, 0xff, 0x80}, got)
}

func TestReceiveTemporaryFileUploadCleansPartialFileOnFailure(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	wantErr := errors.New("stream interrupted")
	call := 0
	err := receiveTemporaryFileUpload(func() (*ypb.UploadToTemporaryFileRequest, error) {
		call++
		if call == 1 {
			return &ypb.UploadToTemporaryFileRequest{Data: []byte("partial")}, nil
		}
		return nil, wantErr
	}, func(*ypb.UploadToTemporaryFileResponse) error {
		return nil
	})
	require.ErrorIs(t, err, wantErr)
	matches, globErr := filepath.Glob(filepath.Join(consts.GetDefaultYakitBaseTempDir(), "fuzztag-upload-*"))
	require.NoError(t, globErr)
	require.Empty(t, matches)
}

func TestReceiveTemporaryFileUploadCleansFileWhenResponseFails(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	wantErr := errors.New("client disconnected before response")
	var uploadedPath string
	err := receiveTemporaryFileUpload(func() (*ypb.UploadToTemporaryFileRequest, error) {
		return nil, io.EOF
	}, func(rsp *ypb.UploadToTemporaryFileResponse) error {
		uploadedPath = rsp.GetFileName()
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	require.NotEmpty(t, uploadedPath)
	require.NoFileExists(t, uploadedPath)
}

func TestUploadToTemporaryFileGRPC(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	client, err := NewLocalClient(true)
	require.NoError(t, err)
	stream, err := client.UploadToTemporaryFile(context.Background())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&ypb.UploadToTemporaryFileRequest{Data: []byte{0x25, 0x50, 0x44}}))
	require.NoError(t, stream.Send(&ypb.UploadToTemporaryFileRequest{Data: []byte{0x46, 0x00, 0xff}}))
	rsp, err := stream.CloseAndRecv()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(rsp.GetFileName()) })
	require.Equal(t, int64(6), rsp.GetSize())
	got, err := os.ReadFile(rsp.GetFileName())
	require.NoError(t, err)
	require.Equal(t, []byte{0x25, 0x50, 0x44, 0x46, 0x00, 0xff}, got)
}

// This is the backend contract behind History/WebFuzzer replacement in remote
// mode: GUI-local bytes are streamed to the engine, the returned engine path is
// placed in {{file(path)}}, and HTTPFuzzer expands that path to the exact file
// bytes on the target wire. A structurally valid ZIP catches text conversion,
// truncation and parser-delimiter bugs that a repeated "aaaa" body cannot.
func TestGRPCMUSTPASS_UploadToTemporaryFile_WebFuzzerReplaysExactZIP(t *testing.T) {
	client := isolateMITMTestSideEffects(t)
	want := readMITMBinaryRepositoryFixture(t, "common", "aireducer", "testdata", "demo.txt.zip")
	requireRealZIPFixture(t, want)

	uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer uploadCancel()
	upload, err := client.UploadToTemporaryFile(uploadCtx)
	require.NoError(t, err)
	for offset := 0; offset < len(want); offset += 997 {
		end := offset + 997
		if end > len(want) {
			end = len(want)
		}
		require.NoError(t, upload.Send(&ypb.UploadToTemporaryFileRequest{Data: want[offset:end]}))
	}
	uploaded, err := upload.CloseAndRecv()
	require.NoError(t, err)
	require.Equal(t, int64(len(want)), uploaded.GetSize())
	require.FileExists(t, uploaded.GetFileName())
	t.Cleanup(func() { _ = os.Remove(uploaded.GetFileName()) })
	stored, err := os.ReadFile(uploaded.GetFileName())
	require.NoError(t, err)
	require.Equal(t, want, stored)

	token := "remote-upload-webfuzzer-" + utils.RandStringBytes(8)
	registerHTTPFlowTokenCleanup(t, token)
	mockCtx, mockCancel := context.WithCancel(context.Background())
	t.Cleanup(mockCancel)
	captured := make(chan []byte, 1)
	host, port := utils.DebugMockHTTPHandlerFuncContext(mockCtx, func(writer http.ResponseWriter, request *http.Request) {
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		select {
		case captured <- bytes.Clone(body):
		default:
		}
		_, writeErr := writer.Write([]byte("ok"))
		require.NoError(t, writeErr)
	})
	target := utils.HostPort(host, port)
	packet := []byte(fmt.Sprintf(
		"POST /%s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/zip\r\nContent-Length: 1\r\nConnection: close\r\n\r\n{{file(%s)}}",
		token,
		target,
		uploaded.GetFileName(),
	))

	fuzzerCtx, fuzzerCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer fuzzerCancel()
	stream, err := client.HTTPFuzzer(fuzzerCtx, &ypb.FuzzerRequest{
		Request:   string(packet),
		ForceFuzz: true,
	})
	require.NoError(t, err)
	for {
		if _, recvErr := stream.Recv(); recvErr != nil {
			break
		}
	}

	select {
	case got := <-captured:
		require.Equal(t, want, got)
		require.NotEqual(t, []byte(lowhttp.ToUnquoteFuzzTagForce(want)), got)
	case <-time.After(3 * time.Second):
		t.Fatal("mock target did not receive the uploaded ZIP through HTTPFuzzer")
	}
}
