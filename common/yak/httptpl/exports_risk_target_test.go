package httptpl

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestBuildRiskTargetAndPacketPairs_SingleResponse(t *testing.T) {
	req := []byte("GET /single HTTP/1.1\r\nHost: example.com\r\n\r\n")
	rsp := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	responses := []*lowhttp.LowhttpResponse{
		{
			Url:        "http://example.com/single",
			RawRequest: req,
			RawPacket:  rsp,
		},
	}

	target, singleReq, singleRsp, rawPairs := buildRiskTargetAndPacketPairs(responses)
	if target != "http://example.com/single" {
		t.Fatalf("unexpected target: %q", target)
	}
	if string(singleReq) != string(req) {
		t.Fatalf("unexpected single request")
	}
	if string(singleRsp) != string(rsp) {
		t.Fatalf("unexpected single response")
	}
	if len(rawPairs) != 0 {
		t.Fatalf("unexpected raw pair count: %d", len(rawPairs))
	}
}

func TestBuildRiskTargetAndPacketPairs_MultiResponse_NoMergedURL(t *testing.T) {
	req1 := []byte("GET /a HTTP/1.1\r\nHost: example.com\r\n\r\n")
	rsp1 := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	req2 := []byte("GET /b HTTP/1.1\r\nHost: example.com\r\n\r\n")
	rsp2 := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	responses := []*lowhttp.LowhttpResponse{
		{
			Url:        "http://example.com/a",
			RawRequest: req1,
			RawPacket:  rsp1,
		},
		{
			Url:        "http://example.com/b",
			RawRequest: req2,
			RawPacket:  rsp2,
		},
	}

	target, singleReq, singleRsp, rawPairs := buildRiskTargetAndPacketPairs(responses)
	if target != "http://example.com/a" {
		t.Fatalf("unexpected target: %q", target)
	}
	if singleReq != nil || singleRsp != nil {
		t.Fatalf("single request/response should be nil in multi-response mode")
	}
	if len(rawPairs) != 2 {
		t.Fatalf("unexpected raw pair count: %d", len(rawPairs))
	}
	if string(rawPairs[0].Req) != string(req1) || string(rawPairs[1].Req) != string(req2) {
		t.Fatalf("raw pair request order changed")
	}
}
