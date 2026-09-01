package nla_test

import (
	"encoding/hex"
	"testing"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
)

// golden 更新说明：NegotiateMessage 现按 NTLMSSP_NEGOTIATE_VERSION 序列化
// （含 8 字节 version 字段），当前基线经 rdp_nla_test.go 的 CredSSP/NTLMv2
// 互操作测试（服务端按 [MS-NLMP] 验证 NTProofStr）确认可用。
func TestEncodeDERTRequest(t *testing.T) {
	ntlm := nla.NewNTLMv2("", "", "")
	result := nla.EncodeDERTRequest([]nla.Message{ntlm.GetNegotiateMessage()}, nil, nil)
	if hex.EncodeToString(result) != "3037a003020102a130302e302ca02a04284e544c4d535350000100000035820860000000000000000000000000000000000000000000000000" {
		t.Error("not equal")
	}
}
