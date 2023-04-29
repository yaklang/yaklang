package t3

import (
	"yaklang/common/yserx"
)

func writeObject(b []byte) []byte {
	return append([]byte("\xfe\x01\x00\x00"), b...)
}

func writeObjectFromJson(json string) []byte {
	if json == "" {
		return []byte("\xfe\x01\x00\x00")
	}
	ser, err := yserx.FromJson([]byte(lookupObj0))
	bobj := yserx.MarshalJavaObjects(ser...)
	if err != nil {
		panic(err)
	}
	return append([]byte("\xfe\x01\x00\x00"), bobj...)
}
