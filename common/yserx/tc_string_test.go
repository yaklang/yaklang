package yserx

import (
	"yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestNewJavaString(t *testing.T) {
	tcString := NewJavaString("testjavastring")
	raw, err := ToJson(tcString)
	if err != nil {
		t.Error(err)
		return
	}
	println(string(raw))
	println(codec.EncodeToHex(tcString.Marshal()))
	objs, err := FromJson(raw)
	if err != nil {
		t.Error(err)
		return
	}
	println(codec.EncodeToHex(objs[0].Marshal()))

	println(codec.EncodeToHex(MarshalJavaObjects(NewJavaString("hello"))))
}
