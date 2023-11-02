package lowhttp

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestNtlmV2(t *testing.T) {
	auth := GetNTLMAuth("test", "test123", "")
	rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: 117.50.163.235\r\n\r\n")), WithLowhttpAuth(auth))
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(rsp)
}

func TestResp(t *testing.T) {
	auth := nla.NewNTLMv2("", "test", "test123")
	auth.GetNegotiateMessage()
	challenge, _ := codec.DecodeBase64("TlRMTVNTUAACAAAAHgAeADgAAAAFAoqibpP+q8pzH8cAAAAAAAAAAJAAkABWAAAACgBjRQAAAA9XAEkATgAtADAANgAxAEUAVgBJAEgASwBEADgANgACAB4AVwBJAE4ALQAwADYAMQBFAFYASQBIAEsARAA4ADYAAQAeAFcASQBOAC0AMAA2ADEARQBWAEkASABLAEQAOAA2AAQAGgAxADAALQA2ADAALQAxADQAOQAtADEANQAxAAMAGgAxADAALQA2ADAALQAxADQAOQAtADEANQAxAAcACACHI0ZLqw3aAQAAAAA=")
	authMessage, _ := auth.GetAuthenticateMessage(challenge)
	authstring := authMessage.Serialize()
	fmt.Println(codec.EncodeBase64(authstring))
}
