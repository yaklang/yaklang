package poc

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestPocWithRandomJA3(t *testing.T) {
	token := utils.RandStringBytes(128)
	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: %d\r\n\r\n%s", len(token), token)))

	for i := 0; i < 16; i++ {
		rsp, _, err := DoGET("http://"+utils.HostPort(host, port), WithRandomJA3(true))
		require.NoError(t, err)
		require.Containsf(t, string(rsp.RawPacket), token, "invalid response")
	}
}

func TestPocRequestWithSession(t *testing.T) {
	token, token2, token3 := utils.RandStringBytes(10), utils.RandStringBytes(10), utils.RandStringBytes(10)
	cookieStr := fmt.Sprintf("%s=%s", token, token2)

	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nConnection: close\r\nSet-Cookie: %s\r\n\r\n", cookieStr)))

	// get cookie from server
	_, _, err := HTTP(fmt.Sprintf(`GET / HTTP/1.1
Host: %s
`, utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)

	// test HTTP / DO
	// if request has cookie
	_, req, err := HTTP(fmt.Sprintf(`GET / HTTP/1.1
Host: %s
`, utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)
	require.Contains(t, string(req), cookieStr)

	_, req2, err := Do(http.MethodGet, fmt.Sprintf("http://%s", utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)
	cookie, err := req2.Cookie(token)
	require.NoError(t, err)
	require.Equal(t, token2, cookie.Value)
}

func TestWithPostParams(t *testing.T) {
	tests := []struct {
		name                string
		input               any
		expectedContentType string
		expectedParams      []string
		description         string
	}{
		{
			name:                "map_input",
			input:               map[string]string{"username": "admin", "password": "123456", "token": "abc123"},
			expectedContentType: "application/x-www-form-urlencoded",
			expectedParams:      []string{"username=admin", "password=123456", "token=abc123"},
			description:         "map input should be converted to form data with correct content type",
		},
		{
			name:                "empty_value_map",
			input:               map[string]string{"username": ""},
			expectedContentType: "application/x-www-form-urlencoded",
			expectedParams:      []string{"username="},
			description:         "empty value map should set content type and empty parameter",
		},
		{
			name:                "mutli_value_map",
			input:               map[string][]string{"username": {"admin", "tom", "jerry"}},
			expectedContentType: "application/x-www-form-urlencoded",
			expectedParams:      []string{"username=admin", "&", "username=tom", "username=jerry"},
			description:         "empty value map should set content type and empty parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				contentType := request.Header.Get("Content-Type")
				body := ""
				if request.Body != nil {
					bodyBytes, _ := io.ReadAll(request.Body)
					body = string(bodyBytes)
				}

				response := fmt.Sprintf("Content-Type: %s\nBody: %s", contentType, body)
				writer.WriteHeader(200)
				writer.Write([]byte(response))
			})

			requestURL := fmt.Sprintf("http://%s", utils.HostPort(host, port))

			rsp, req, err := DoPOST(requestURL, WithPostParams(tt.input))
			require.NoError(t, err, tt.description)
			require.NotNil(t, rsp, "Response should not be nil")
			require.NotNil(t, req, "Request should not be nil")

			t.Logf("raw packet:%s", rsp.RawRequest)

			if tt.expectedContentType != "" {
				require.Equal(t, tt.expectedContentType, req.Header.Get("Content-Type"))
			}

			require.NoError(t, err, "Should be able to parse form data")

			if tt.expectedParams != nil {
				for _, param := range tt.expectedParams {
					require.Contains(t, string(rsp.RawRequest), param)
				}
			}

			t.Logf("✓ %s: %s", tt.name, tt.description)
		})
	}
}

func TestExtractPostParams(t *testing.T) {
	type args struct {
		raw []byte
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "form_urlencoded_should_decode_plus",
			args: args{
				raw: []byte("POST /login HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 17\r\n\r\na=1&b=hello+world"),
			},
			want: map[string]string{
				"a": "1",
				"b": "hello+world",
			},
		},
		{
			name: "json_string_should_not_parse_as_query",
			args: args{
				raw: []byte("HTTP/1.1 403 Forbidden\r\nContent-Type: application/json\r\nContent-Length: 13\r\n\r\n\"aaaaa+bbbbb\""),
			},
			want:    map[string]string{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractPostParams(tt.args.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractPostParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractPostParams() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDownload_Basic(t *testing.T) {
	// Create mock HTTP server with file content
	fileContent := []byte("Hello, this is a test file content for download testing!")
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
		writer.WriteHeader(200)
		writer.Write(fileContent)
	})

	// Create temp directory for download
	tmpDir, err := os.MkdirTemp("", "poc_download_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Download file
	filePath, err := Download(
		fmt.Sprintf("http://%s/testfile.txt", utils.HostPort(host, port)),
		WithDownloadDir(tmpDir),
	)
	require.NoError(t, err)
	require.NotEmpty(t, filePath)

	// Verify file exists and has correct content
	downloadedContent, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, fileContent, downloadedContent)
}

func TestDownload_WithProgress(t *testing.T) {
	// Create mock HTTP server with larger file content
	fileContent := []byte(utils.RandStringBytes(1024 * 10)) // 10KB
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
		writer.WriteHeader(200)
		writer.Write(fileContent)
	})

	tmpDir, err := os.MkdirTemp("", "poc_download_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	var progressCalled int32
	var lastPercent float64

	filePath, err := Download(
		fmt.Sprintf("http://%s/largefile.bin", utils.HostPort(host, port)),
		WithDownloadDir(tmpDir),
		WithDownloadProgress(func(downloaded, total int64, percent float64) {
			atomic.AddInt32(&progressCalled, 1)
			lastPercent = percent
		}),
	)
	require.NoError(t, err)
	require.NotEmpty(t, filePath)

	// Verify progress callback was called
	require.Greater(t, atomic.LoadInt32(&progressCalled), int32(0), "progress callback should be called")
	require.Equal(t, 100.0, lastPercent, "last progress should be 100%")

	// Verify file content
	downloadedContent, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, fileContent, downloadedContent)
}

func TestDownload_WithFinishedCallback(t *testing.T) {
	fileContent := []byte("Finished callback test content")
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
		writer.WriteHeader(200)
		writer.Write(fileContent)
	})

	tmpDir, err := os.MkdirTemp("", "poc_download_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	var finishedFilePath string
	filePath, err := Download(
		fmt.Sprintf("http://%s/callback.txt", utils.HostPort(host, port)),
		WithDownloadDir(tmpDir),
		WithDownloadFinished(func(fp string) {
			finishedFilePath = fp
		}),
	)
	require.NoError(t, err)
	require.Equal(t, filePath, finishedFilePath, "finished callback should receive correct file path")
}

func TestDownload_FilenameFromContentDisposition(t *testing.T) {
	fileContent := []byte("Content disposition test")
	expectedFilename := "custom_name.dat"
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", expectedFilename))
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
		writer.WriteHeader(200)
		writer.Write(fileContent)
	})

	tmpDir, err := os.MkdirTemp("", "poc_download_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	filePath, err := Download(
		fmt.Sprintf("http://%s/download", utils.HostPort(host, port)),
		WithDownloadDir(tmpDir),
	)
	require.NoError(t, err)
	require.Equal(t, expectedFilename, filepath.Base(filePath), "filename should be extracted from Content-Disposition")
}

func TestDownload_CustomFilename(t *testing.T) {
	fileContent := []byte("Custom filename test")
	customFilename := "my_custom_file.txt"
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
		writer.WriteHeader(200)
		writer.Write(fileContent)
	})

	tmpDir, err := os.MkdirTemp("", "poc_download_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	filePath, err := Download(
		fmt.Sprintf("http://%s/anypath", utils.HostPort(host, port)),
		WithDownloadDir(tmpDir),
		WithDownloadFilename(customFilename),
	)
	require.NoError(t, err)
	require.Equal(t, customFilename, filepath.Base(filePath), "should use custom filename")
}

func TestDownload_FilenameFromURL(t *testing.T) {
	fileContent := []byte("URL filename test")
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
		writer.WriteHeader(200)
		writer.Write(fileContent)
	})

	tmpDir, err := os.MkdirTemp("", "poc_download_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	filePath, err := Download(
		fmt.Sprintf("http://%s/path/to/myfile.zip", utils.HostPort(host, port)),
		WithDownloadDir(tmpDir),
	)
	require.NoError(t, err)
	require.Equal(t, "myfile.zip", filepath.Base(filePath), "filename should be extracted from URL path")
}

func TestDownloadWithMethod_POST(t *testing.T) {
	fileContent := []byte("POST download test")
	var receivedMethod string
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedMethod = request.Method
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
		writer.WriteHeader(200)
		writer.Write(fileContent)
	})

	tmpDir, err := os.MkdirTemp("", "poc_download_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	filePath, err := DownloadWithMethod(
		"POST",
		fmt.Sprintf("http://%s/download", utils.HostPort(host, port)),
		WithDownloadDir(tmpDir),
	)
	require.NoError(t, err)
	require.NotEmpty(t, filePath)
	require.Equal(t, "POST", receivedMethod, "should use POST method")

	// Verify file content
	downloadedContent, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, fileContent, downloadedContent)
}

func TestExtractFilenameHelpers(t *testing.T) {
	// Test extractFilenameFromURL
	tests := []struct {
		url      string
		expected string
	}{
		{"http://example.com/file.txt", "file.txt"},
		{"http://example.com/path/to/document.pdf", "document.pdf"},
		{"http://example.com/", ""},
		{"http://example.com", ""},
		{"http://example.com/dir/", ""},
	}

	for _, tt := range tests {
		result := extractFilenameFromURL(tt.url)
		require.Equal(t, tt.expected, result, "extractFilenameFromURL(%s)", tt.url)
	}
}

func TestExtractFilenameFromHeader(t *testing.T) {
	tests := []struct {
		header   string
		expected string
	}{
		{"attachment; filename=\"test.zip\"", "test.zip"},
		{"attachment; filename=test.zip", "test.zip"},
		{"attachment; filename=\"path/to/file.txt\"", "file.txt"},
		{"inline; filename=\"document.pdf\"", "document.pdf"},
		{"", ""},
		{"attachment", ""},
	}

	for _, tt := range tests {
		headerBytes := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Disposition: %s\r\n\r\n", tt.header))
		result := extractFilenameFromHeader(headerBytes)
		require.Equal(t, tt.expected, result, "extractFilenameFromHeader for: %s", tt.header)
	}
}

func TestExtractContentLength(t *testing.T) {
	tests := []struct {
		header   string
		expected int64
	}{
		{"Content-Length: 1234", 1234},
		{"Content-Length: 0", 0},
		{"Content-Length: 999999999", 999999999},
		{"", -1},
		{"Content-Length: invalid", -1},
	}

	for _, tt := range tests {
		headerBytes := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\n%s\r\n\r\n", tt.header))
		result := extractContentLength(headerBytes)
		require.Equal(t, tt.expected, result, "extractContentLength for: %s", tt.header)
	}
}
