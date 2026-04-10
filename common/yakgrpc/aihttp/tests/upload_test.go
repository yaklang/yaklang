package aihttp_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/aihttp"
)

func TestUploadFile(t *testing.T) {
	uploadDir := t.TempDir()
	gw := newTestGateway(t, aihttp.WithUploadDir(uploadDir))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "notes.txt")
	require.NoError(t, err)
	_, err = fileWriter.Write([]byte("hello aihttp"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", "/agent/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := performRequest(gw, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp aihttp.UploadFileResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "notes.txt", resp.OriginalName)
	require.Equal(t, int64(len("hello aihttp")), resp.Size)
	require.Equal(t, uploadDir, filepath.Dir(resp.Path))

	data, err := os.ReadFile(resp.Path)
	require.NoError(t, err)
	require.Equal(t, "hello aihttp", string(data))
}

func TestGatewayUploadDirOption(t *testing.T) {
	uploadDir := t.TempDir()
	gw := newTestGateway(t, aihttp.WithUploadDir(uploadDir))
	require.Equal(t, uploadDir, gw.GetUploadDir())
}
