package msrdp

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

func CalcNTLMHash(password, user, domain string) ([]byte, []byte) {
	key := MD4(UnicodeEncode(password))
	nt := codec.HmacMD5(key, UnicodeEncode(strings.ToUpper(user)+domain))
	return nt, nt
}
func CalcNetNTLMHash(challengeData map[string]any, block []byte, nt, lm []byte) ([]byte, []byte, []byte) {
	serverChallenge := challengeData["ServerChallenge"].([]byte)
	clientChallenge := []byte(utils.RandStringBytes(8))
	infoFields := challengeData["TargetInfoFields"].(map[string]any)
	offset := infoFields["BufferOffset"].(uint32) - 56 // header length is 65
	length := infoFields["Length"].(uint16)
	serverInfo := readFieldFromBlock(block, offset, length)
	serverInfoMap := map[uint16][]byte{}
	infoReader := bytes.NewReader(serverInfo)
	for {
		v, err := ParseRdpSubProtocol(infoReader, "AV_PAIR")
		if err != nil {
			panic(err)
		}
		pair := v.(map[string]any)
		if pair["AvId"].(uint16) == 0x0000 {
			break
		}
		if pair["AvLen"].(uint16) > 0 {
			serverInfoMap[pair["AvId"].(uint16)] = pair["Value"].([]byte)
		} else {
			serverInfoMap[pair["AvId"].(uint16)] = []byte{}
		}
	}

	tempBuff := &bytes.Buffer{}
	tempBuff.Write([]byte{0x01, 0x01}) // Responser version, HiResponser version
	tempBuff.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	tempBuff.Write(serverInfoMap[0x0007]) // server timestamp
	tempBuff.Write(clientChallenge)
	tempBuff.Write([]byte{0x00, 0x00, 0x00, 0x00})
	tempBuff.Write(serverInfo) // server name
	tempBuff.Write([]byte{0x00, 0x00, 0x00, 0x00})
	ntClientChallenge := tempBuff.Bytes()

	ntProof := codec.HmacMD5(nt, append(serverChallenge, ntClientChallenge...))
	ntChallResp := append(ntProof, ntClientChallenge...)

	lmChallResp := codec.HmacMD5(lm, append(serverChallenge, clientChallenge...))
	lmChallResp = append(lmChallResp, clientChallenge...)

	SessBaseKey := codec.HmacMD5(nt, ntProof)
	return ntChallResp, lmChallResp, SessBaseKey
}
