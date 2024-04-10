package ppp

import (
	"bytes"
	"crypto/md5"
	"github.com/yaklang/yaklang/common/utils"
)

func GenerateCHAPResponse(id, challenge, username, password, autype []byte) ([]byte, error) {

	if bytes.Equal(CHAP_MD5, autype) {
		return GenerateCHAPMD5Response(id, password, challenge), nil
	}
	if bytes.Equal(MS_CHAP_V2, autype) {
		resp, err := GenerateMSChapV2Response(challenge, username, password)
		return resp, err
	}
	return nil, utils.Error("no support auth type")
}

func GenerateCHAPMD5Response(id, password, challenge []byte) []byte {
	toBuf := append(id, password...)
	toBuf = append(toBuf, challenge...)
	return utils.InterfaceToBytes(md5.Sum(toBuf))
}
