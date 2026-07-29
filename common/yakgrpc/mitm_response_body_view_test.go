//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestMITMMirrorResponseBodyViewAndAsyncHookSnapshot(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 7\r\n\r\npayload")
	legacyHeader, legacyBody := lowhttp.SplitHTTPPacketFast(packet)

	header, bodyView := splitMITMMirrorResponseView(packet)
	if header != legacyHeader || !bytes.Equal(bodyView, legacyBody) {
		t.Fatalf("mirror response split changed: header=%q body=%q", header, bodyView)
	}
	bodyOffset := bytes.Index(packet, []byte("\r\n\r\n")) + 4
	if len(bodyView) == 0 || &bodyView[0] != &packet[bodyOffset] {
		t.Fatal("synchronous mirror body is not a view of the plain response")
	}

	snapshot := snapshotMITMMirrorHookBody(bodyView)
	if !bytes.Equal(snapshot, bodyView) {
		t.Fatalf("unexpected async hook snapshot: %q", snapshot)
	}
	packet[len(packet)-1] = 'D'
	if string(snapshot) != "payload" {
		t.Fatalf("async hook snapshot aliases the plain response: %q", snapshot)
	}
	snapshot[0] = 'P'
	if packet[bodyOffset] != 'p' {
		t.Fatal("plain response aliases the async hook snapshot")
	}
}
