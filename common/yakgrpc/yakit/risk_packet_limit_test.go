package yakit

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

func TestRiskPacketLimitPreservesBinaryPrefix(t *testing.T) {
	packet := append([]byte("POST /upload HTTP/1.1\r\nHost: example.test\r\n\r\n"), 0xff, 0x80, 0x00)
	packet = append(packet, bytes.Repeat([]byte("x"), MaxSize)...)
	want := string(packet[:MaxSize-3]) + "..."

	var record schema.Risk
	WithRiskParam_Request(packet)(&record)
	WithRiskParam_Response(packet)(&record)
	for name, quoted := range map[string]string{
		"request":  record.QuotedRequest,
		"response": record.QuotedResponse,
	} {
		got, err := strconv.Unquote(quoted)
		if err != nil {
			t.Fatalf("unquote %s: %v", name, err)
		}
		if got != want {
			t.Fatalf("%s lost binary bytes before the risk packet limit: got %d bytes, want %d", name, len(got), len(want))
		}
	}
}
