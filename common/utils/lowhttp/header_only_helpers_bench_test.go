package lowhttp

import (
	"bytes"
	"reflect"
	"testing"
)

func largeHeaderOnlyHelperPacket() []byte {
	body := bytes.Repeat([]byte("0123456789abcdef"), 16*1024)
	packet := make([]byte, 0, len(body)+256)
	packet = append(packet, "HTTP/1.1 207 Multi-Status\r\n"...)
	packet = append(packet, "Content-Type: application/octet-stream\r\n"...)
	packet = append(packet, "Set-Cookie: token=first\r\n"...)
	packet = append(packet, "Set-Cookie: token=second\r\n"...)
	packet = append(packet, "X-Test: one\r\n"...)
	packet = append(packet, "X-Test: two\r\n"...)
	packet = append(packet, "Content-Length: 262144\r\n\r\n"...)
	packet = append(packet, body...)
	return packet
}

func getHTTPPacketHeadersWithBodyCopy(packet []byte) map[string]string {
	result := make(map[string]string)
	SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if key, value := SplitHTTPHeader(line); key != "" {
			result[key] = value
		}
		return line
	})
	return result
}

func getStatusCodeFromResponseWithBodyCopy(packet []byte) (statusCode int) {
	SplitHTTPPacket(packet, nil, func(_ string, code int, _ string) error {
		statusCode = code
		return nil
	})
	return statusCode
}

func TestHeaderOnlyHelpersMatchBodyCopyPath(t *testing.T) {
	packet := largeHeaderOnlyHelperPacket()
	if got, want := GetHTTPPacketHeaders(packet), getHTTPPacketHeadersWithBodyCopy(packet); !reflect.DeepEqual(got, want) {
		t.Fatalf("headers differ: got %#v, want %#v", got, want)
	}
	if got, want := GetStatusCodeFromResponse(packet), getStatusCodeFromResponseWithBodyCopy(packet); got != want {
		t.Fatalf("status differs: got %d, want %d", got, want)
	}
	if got := GetHTTPPacketContentType(packet); got != "application/octet-stream" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if got := GetHTTPPacketCookieValues(packet, "token"); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("unexpected cookies: %#v", got)
	}
}

func BenchmarkHeaderOnlyHelpersLargeBody(b *testing.B) {
	packet := largeHeaderOnlyHelperPacket()
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()

	b.Run("headers_body_copy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = getHTTPPacketHeadersWithBodyCopy(packet)
		}
	})
	b.Run("headers_body_view", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = GetHTTPPacketHeaders(packet)
		}
	})
	b.Run("status_body_copy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = getStatusCodeFromResponseWithBodyCopy(packet)
		}
	})
	b.Run("status_body_view", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = GetStatusCodeFromResponse(packet)
		}
	})
}
