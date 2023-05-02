package yserx

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"yaklang/common/log"
	"yaklang/common/yak/yaklib/codec"
)

func TestParseJavaSerialized_1(t *testing.T) {
	test := assert.New(t)

	payload := payload0

	var (
		r   []JavaSerializable
		err error
	)

	r, err = ParseJavaSerialized(payload)
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	_ = r
	raw2 := MarshalJavaObjects(r...)
	s1 := string(payload)
	s2 := string(raw2)

	res, err := ParseJavaSerialized(raw2)
	if err != nil {
		log.Errorf(err.Error())
		panic(1)
		return
	}
	_ = res

	println(codec.Md5(s1))
	println(codec.Md5(s2))

	println(strconv.Quote(s1))
	println(strconv.Quote(s2))

	// \x00\x03[[B[\x00\x06_class
	println(codec.EncodeToHex(payload[:]))
	println(codec.EncodeToHex(MarshalJavaObjects(r...)))

	for _, r := range res {
		raw, err := ToJson(r)
		if err != nil {
			continue
		}
		println(string(raw))
		obj, err := FromJson(raw)
		if err != nil {
			panic(err)
		}
		_ = obj
		raw2, err = ToJson(obj[0])
		if !bytes.Equal(raw, raw2) {
			println(string(raw))
			println(string(raw2))
			panic("from json error")
		}

	}
}
