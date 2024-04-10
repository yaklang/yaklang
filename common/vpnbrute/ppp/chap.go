package ppp

import (
	"crypto/md5"
	"github.com/yaklang/yaklang/common/utils"
)

func GenerateCHAPResponse(id, challenge, username, password, autype []byte) ([]byte, error) {
	switch autype {
	case CHAP_MD5:
		return GenerateCHAPMD5Response(id, password, challenge), nil
	case MS_CHAP_V2:
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
