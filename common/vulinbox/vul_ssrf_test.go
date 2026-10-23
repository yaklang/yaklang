package vulinbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func startSSRFScenarioServer(t *testing.T) string {
	t.Helper()
	// 不用 t.TempDir：gorm 连接池持有 sqlite 文件，Windows 下强制清理会让测试失败
	dir, err := os.MkdirTemp("", "vulinbox-ssrf-test-*")
	require.NoError(t, err)
	t.Setenv("YAKIT_HOME", dir)
	t.Cleanup(func() { os.RemoveAll(dir) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	addr, err := NewVulinServerEx(ctx, true, false, "127.0.0.1")
	require.NoError(t, err)
	return addr
}

func TestVulinboxSSRFPostLevels(t *testing.T) {
	base := startSSRFScenarioServer(t)
	flagURL := base + "/ssrf/flag"

	postBody := func(t *testing.T, path, contentType, body string) *http.Response {
		t.Helper()
		rsp, err := http.Post(base+path, contentType, strings.NewReader(body))
		require.NoError(t, err)
		t.Cleanup(func() { rsp.Body.Close() })
		return rsp
	}

	readBody := func(t *testing.T, rsp *http.Response) string {
		t.Helper()
		raw, err := io.ReadAll(rsp.Body)
		require.NoError(t, err)
		return string(raw)
	}

	t.Run("in-post-multipart echoes flag from url field", func(t *testing.T) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(t, w.WriteField("url", flagURL))
		require.NoError(t, w.Close())
		rsp := postBody(t, "/ssrf/in-post-multipart", w.FormDataContentType(), buf.String())
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.Contains(t, readBody(t, rsp), "flag{")
	})

	t.Run("in-post-multipart form posts to itself", func(t *testing.T) {
		rsp, err := http.Get(base + "/ssrf/in-post-multipart")
		require.NoError(t, err)
		t.Cleanup(func() { rsp.Body.Close() })
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.Contains(t, readBody(t, rsp), `action="/ssrf/in-post-multipart"`)
	})

	t.Run("in-post-multipart empty url", func(t *testing.T) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(t, w.WriteField("url", ""))
		require.NoError(t, w.Close())
		rsp := postBody(t, "/ssrf/in-post-multipart", w.FormDataContentType(), buf.String())
		require.Contains(t, readBody(t, rsp), "暂无数据")
	})

	t.Run("in-json-body echoes flag from url key", func(t *testing.T) {
		rsp := postBody(t, "/ssrf/in-json-body", "application/json",
			fmt.Sprintf(`{"url": %q}`, flagURL))
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.Contains(t, readBody(t, rsp), "flag{")
	})

	t.Run("in-json-body accepts ref key fallback", func(t *testing.T) {
		rsp := postBody(t, "/ssrf/in-json-body", "application/json",
			fmt.Sprintf(`{"ref": %q}`, flagURL))
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.Contains(t, readBody(t, rsp), "flag{")
	})

	t.Run("in-json-body empty body", func(t *testing.T) {
		rsp := postBody(t, "/ssrf/in-json-body", "application/json", "")
		require.Contains(t, readBody(t, rsp), "暂无数据")
	})

	t.Run("in-json-body invalid json", func(t *testing.T) {
		rsp := postBody(t, "/ssrf/in-json-body", "application/json", "{not-json")
		require.Contains(t, readBody(t, rsp), "JSON Syntax Error")
	})

	t.Run("json-in-post-param echoes flag from json param", func(t *testing.T) {
		body := "json=" + url.QueryEscape(fmt.Sprintf(`{"url": %q}`, flagURL))
		rsp := postBody(t, "/ssrf/json-in-post-param",
			"application/x-www-form-urlencoded", body)
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.Contains(t, readBody(t, rsp), "flag{")
	})

	t.Run("json-in-post-param empty param", func(t *testing.T) {
		rsp := postBody(t, "/ssrf/json-in-post-param",
			"application/x-www-form-urlencoded", "json=")
		require.Contains(t, readBody(t, rsp), "暂无数据")
	})

	t.Run("json-in-post-param invalid json", func(t *testing.T) {
		rsp := postBody(t, "/ssrf/json-in-post-param",
			"application/x-www-form-urlencoded", "json="+url.QueryEscape("{not-json"))
		require.Contains(t, readBody(t, rsp), "JSON Syntax Error")
	})
}
