package codec

import "github.com/yaklang/yaklang/common/gmsm/sm3"

func SM3(raw interface{}) []byte {
	return sm3.Sm3Sum(interfaceToBytes(raw))
}
