package har

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

//go:embed testdata/example.com.har
var testHARBytes []byte

func TestImportHTTPArchive(t *testing.T) {
	buf := bytes.NewBuffer(testHARBytes)
	count := 0
	ImportHTTPArchiveStream(buf, func(entry *HAREntry) error {
		count++
		req, rsp := entry.Request, entry.Response
		require.Equal(t, "GET", req.Method)
		require.Equal(t, "https://example.com/", req.URL)
		require.Equal(t, "http/2.0", req.HTTPVersion)
		require.Len(t, req.Headers, 19)
		require.Equal(t, 200, rsp.StatusCode)
		require.Equal(t, "", rsp.StatusText)
		require.Equal(t, "http/2.0", rsp.HTTPVersion)
		require.Len(t, rsp.Headers, 9)

		return nil
	})
	require.Equal(t, 1, count, "should have 1 entry")
}

func TestExportHTTPArchive(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	ch := make(chan *HAREntry)
	go func() {
		ch <- &HAREntry{
			Request: &HARRequest{
				Method:      "GET",
				URL:         "https://www.example.com",
				HTTPVersion: "http/2.0",
				Headers: []*HARKVPair{
					{
						Name:  ":authority",
						Value: "example.com",
					},
					{
						Name:  ":method",
						Value: "GET",
					},
					{
						Name:  ":path",
						Value: "/",
					},
					{
						Name:  ":scheme",
						Value: "https",
					},
					{
						Name:  "user-agent",
						Value: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
					},
				},
				HeadersSize: -1,
				BodySize:    0,
			},
			Response: &HARResponse{},
		}
		close(ch)
	}()

	err := ExportHTTPArchiveStream(buf, &HTTPArchive{
		Log: &Log{
			Entries: &Entries{
				entriesChannel: ch,
			},
		},
	})
	require.NoError(t, err)

	got := buf.Bytes()
	result := gjson.ParseBytes(got)
	resultLog := result.Get("log")
	resultEntries := resultLog.Get("entries")
	entries := resultEntries.Array()
	require.True(t, resultLog.Exists())
	require.True(t, resultEntries.Exists())
	require.True(t, len(entries) > 0)
	req := entries[0].Get("request")
	require.True(t, req.Exists())
	require.Equal(t, "GET", req.Get("method").String())
	require.Equal(t, "https://www.example.com", req.Get("url").String())
	require.Equal(t, "http/2.0", req.Get("httpVersion").String())
	require.Equal(t, int64(-1), req.Get("headersSize").Int())
	require.Equal(t, int64(0), req.Get("bodySize").Int())
	resultHeaders := req.Get("headers")
	require.True(t, resultHeaders.Exists())
	headers := resultHeaders.Array()
	require.Len(t, headers, 5)
}

func TestSmokeImportAndExport(t *testing.T) {
	dir := t.TempDir()
	har, err := ImportHTTPArchive(bytes.NewBuffer(testHARBytes))
	require.NoError(t, err)
	p := path.Join(dir, fmt.Sprintf("%s.har", uuid.NewString()))
	fh, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0644)
	require.NoError(t, err)
	err = ExportHTTPArchiveStream(fh, har)
	require.NoError(t, err)
}
