package yso

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yserx"
)

func RepClassName(echoTmplClass []byte, oldN string, newN string) []byte {
	//查找出所有字符串的位置
	var poss []int
	start := 0
	for i := 0; i < 3; i++ {
		pos := IndexFromBytes(echoTmplClass[start:], oldN)
		if pos == -1 {
			break
		}
		poss = append(poss, pos+start)
		start += (pos + len(oldN))
	}

	Bytes2Int := func(b []byte) int {
		return int(b[0])<<8 + int(b[1])
	}

	ll := len(oldN)
	var buffer bytes.Buffer

	//分别对三种情况做替换
	pre := 0
	for _, pos := range poss {
		if string(echoTmplClass[pos-1]) == "L" {
			buffer.Write(echoTmplClass[pre : pos-3])
			buffer.Write(yserx.IntTo2Bytes(len(newN) + 2))
			buffer.Write([]byte("L" + newN))
			pre = pos + len(oldN)
		} else {
			l := Bytes2Int(echoTmplClass[pos-2 : pos])
			if l == ll+5 {
				buffer.Write(echoTmplClass[pre : pos-2])
				buffer.Write(yserx.IntTo2Bytes(len(newN) + 5))
				buffer.Write([]byte(newN))
				pre = pos + len(oldN)
				//buffer.Write(echoTmplClass[pos+len(oldN):])
			} else if l == ll {
				buffer.Write(echoTmplClass[pre : pos-2])
				buffer.Write(yserx.IntTo2Bytes(len(newN)))
				buffer.Write([]byte(newN))
				pre = pos + len(oldN)
			}
		}

	}
	buffer.Write(echoTmplClass[pre:])
	res := buffer.Bytes()
	return res
}
func RepCmd(echoTmplClass []byte, zw string, cmd string) []byte {
	pos := IndexFromBytes(echoTmplClass, zw)
	var buffer bytes.Buffer
	buffer.Write(echoTmplClass[:pos-2])
	buffer.Write(yserx.IntTo2Bytes(len(cmd)))
	buffer.Write([]byte(cmd))
	buffer.Write(echoTmplClass[pos+len(zw):])
	echoTmplClassRep := buffer.Bytes()
	return echoTmplClassRep
}

func IndexFromBytes(byt []byte, sub interface{}) int {
	return bytes.Index(byt, utils.InterfaceToBytes(sub))
}
