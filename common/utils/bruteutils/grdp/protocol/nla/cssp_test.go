package nla_test

import (
	"encoding/hex"
	"testing"

	"yaklang/common/utils/bruteutils/grdp/protocol/nla"
)

func TestEncodeDERTRequest(t *testing.T) {
	ntlm := nla.NewNTLMv2("", "", "")
	result := nla.EncodeDERTRequest([]nla.Message{ntlm.GetNegotiateMessage()}, nil, nil)
	if hex.EncodeToString(result) != "302fa003020102a12830263024a02204204e544c4d53535000010000003582086000000000000000000000000000000000" {
		t.Error("not equal")
	}
}
