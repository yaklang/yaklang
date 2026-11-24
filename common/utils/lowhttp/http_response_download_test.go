package lowhttp

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFixHTTPResponsePreservesDownloadContentTypes(t *testing.T) {
	xlsx, err := os.ReadFile("../fileparser/resources/static/test.xlsx")
	require.NoError(t, err)
	payloads := map[string]struct {
		body         []byte
		detectedType string
	}{
		// MIME signature fixture; full PDF rendering is not exercised here.
		"pdf":  {[]byte("%PDF-1.4\n%\xff\xfe\n%%EOF\n"), "application/pdf"},
		"xlsx": {xlsx, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	}
	fixers := map[string]func([]byte) ([]byte, error){
		"response": func(raw []byte) ([]byte, error) {
			fixed, body, err := FixHTTPResponse(raw)
			_, packetBody := SplitHTTPHeadersAndBodyFromPacketView(fixed)
			if err == nil && !bytes.Equal(packetBody, body) {
				return nil, fmt.Errorf("returned body differs from fixed packet body")
			}
			return fixed, err
		},
		"packet":   FixHTTPResponsePacket,
		"borrowed": FixHTTPResponsePacketBorrowed,
	}
	const disposition = `attachment; filename="report"`
	buildResponse := func(contentType string, payload []byte) []byte {
		// Deliberately stale length: preserving the type must not disable framing repair.
		return append([]byte("HTTP/1.1 200 OK\r\nContent-Type: "+contentType+"\r\nContent-Disposition: "+disposition+"\r\nContent-Length: 1\r\n\r\n"), payload...)
	}
	// Exercise the same payloads through the normal repair path, so a detector
	// regression cannot make the preservation assertions below pass vacuously.
	for name, payload := range payloads {
		for api, fix := range fixers {
			t.Run("sniff-control/"+name+"/"+api, func(t *testing.T) {
				raw := buildResponse("application/octet-stream", payload.body)
				fixed, err := fix(raw)
				require.NoError(t, err)
				require.Equal(t, payload.detectedType, GetHTTPPacketHeader(fixed, "Content-Type"))
				_, body := SplitHTTPHeadersAndBodyFromPacketView(fixed)
				require.Equal(t, payload.body, body)
			})
		}
	}
	for _, contentType := range []string{
		"application/x-download",
		`Application/X-Download; name="report;2026.pdf"`,
		"APPLICATION/X-DOWNLOAD; charset=GBK",
		"application/force-download",
		`Application/Force-Download; name="report;2026.pdf"`,
		"APPLICATION/FORCE-DOWNLOAD; charset=GBK",
	} {
		for name, sample := range payloads {
			payload := sample.body
			for api, fix := range fixers {
				t.Run(contentType+"/"+name+"/"+api, func(t *testing.T) {
					raw := buildResponse(contentType, payload)
					before := bytes.Clone(raw)
					fixed, err := fix(raw)
					require.NoError(t, err)
					require.Equal(t, contentType, GetHTTPPacketHeader(fixed, "Content-Type"))
					require.Equal(t, disposition, GetHTTPPacketHeader(fixed, "Content-Disposition"))
					require.Equal(t, strconv.Itoa(len(payload)), GetHTTPPacketHeader(fixed, "Content-Length"))
					_, body := SplitHTTPHeadersAndBodyFromPacketView(fixed)
					require.Equal(t, payload, body)
					require.Equal(t, before, raw, "input must remain immutable")
					again, err := fix(fixed)
					require.NoError(t, err)
					require.Equal(t, fixed, again, "repeated rule/storage normalization must retain the type")
				})
			}
		}
	}
}

func TestFixHTTPResponseNoSniffStillDecodesEncodings(t *testing.T) {
	payload := []byte("%PDF-1.4\n%\xff\xfe\n%%EOF\n")
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	chunked := append([]byte(fmt.Sprintf("%x\r\n", compressed.Len())), compressed.Bytes()...)
	chunked = append(chunked, []byte("\r\n0\r\n\r\n")...)
	for _, contentType := range []string{"application/x-download", "application/force-download"} {
		t.Run(contentType, func(t *testing.T) {
			raw := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: "+contentType+"\r\nContent-Encoding: gzip\r\nTransfer-Encoding: chunked\r\n\r\n"), chunked...)
			before := bytes.Clone(raw)
			for _, fix := range []func([]byte) ([]byte, error){FixHTTPResponsePacket, FixHTTPResponsePacketBorrowed} {
				fixed, err := fix(raw)
				require.NoError(t, err)
				require.Equal(t, contentType, GetHTTPPacketHeader(fixed, "Content-Type"))
				require.Empty(t, GetHTTPPacketHeader(fixed, "Content-Encoding"))
				require.Empty(t, GetHTTPPacketHeader(fixed, "Transfer-Encoding"))
				require.Equal(t, strconv.Itoa(len(payload)), GetHTTPPacketHeader(fixed, "Content-Length"))
				_, body := SplitHTTPHeadersAndBodyFromPacketView(fixed)
				require.Equal(t, payload, body)
				require.Equal(t, before, raw)
			}
		})
	}
}

func TestFixHTTPResponseNoSniffKeepsOtherMIMERepairs(t *testing.T) {
	payload := []byte("%PDF-1.4\n%\xff\xfe\n%%EOF\n")
	for _, contentType := range []string{
		"",
		"text/html; charset=GBK",
		"application/unknown",
		"application/x-download-custom",
		"application/octet-stream",
		"APPLICATION/OCTET-STREAM; charset=GBK",
		"application/force-download-custom",
	} {
		t.Run(contentType, func(t *testing.T) {
			header := "HTTP/1.1 200 OK\r\n"
			if contentType != "" {
				header += "Content-Type: " + contentType + "\r\n"
			}
			raw := append([]byte(header+"\r\n"), payload...)
			fixed, body, err := FixHTTPResponse(raw)
			require.NoError(t, err)
			require.Equal(t, "application/pdf", GetHTTPPacketHeader(fixed, "Content-Type"))
			require.Equal(t, payload, body)
		})
	}
}

func TestLowhttpNoSniffContentTypeMetadata(t *testing.T) {
	payload := []byte("%PDF-1.4\n%\xff\xfe\n%%EOF\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", r.URL.Query().Get("type"))
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	for _, contentType := range []string{"application/x-download", "application/octet-stream", "application/force-download", "text/html"} {
		t.Run(contentType, func(t *testing.T) {
			packet := []byte(fmt.Sprintf("GET /?type=%s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", contentType, server.Listener.Addr()))
			rsp, err := HTTPWithoutRedirect(WithPacketBytes(packet), WithSaveHTTPFlow(false))
			require.NoError(t, err)
			require.Equal(t, contentType, rsp.OriginContentType)
			require.Equal(t, contentType, GetHTTPPacketHeader(rsp.BareResponse, "Content-Type"))
			wantType := contentType
			if contentType != "application/x-download" && contentType != "application/force-download" {
				wantType = "application/pdf"
				require.True(t, rsp.IsFixContentType)
				require.Equal(t, wantType, rsp.FixContentType)
			} else {
				require.False(t, rsp.IsFixContentType)
				require.Empty(t, rsp.FixContentType)
			}
			require.True(t, rsp.ResponsePacketFixed, "packet normalization still runs")
			require.Equal(t, wantType, GetHTTPPacketHeader(rsp.RawPacket, "Content-Type"))
			_, body := SplitHTTPHeadersAndBodyFromPacketView(rsp.RawPacket)
			require.Equal(t, payload, body)
		})
	}
}
