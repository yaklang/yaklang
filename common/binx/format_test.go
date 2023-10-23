package binx

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestFormat(t *testing.T) {
	results, err := BinaryRead(
		bytes.NewBufferString("\x33\x22\x80\xff\xff\x03aaa"),
		NewInt16("ccc"),
		NewUint8("bbb"),
		NewUint16("ddd"),
		NewUint8("eee"),
		NewBuffer("value", "eee"),
	)
	if err != nil {
		t.Fatal(err)
	}
	test := assert.New(t)
	test.Equal(results[0].GetBytes(), []byte("\x33\x22"))
	test.Equal(results[1].GetBytes(), []byte("\x80"))
	test.Equal(results[1].GetBytes(), []byte("\x80"))

	ret := results[0].AsInt16()
	test.Equal(ret, int16(0x3322))
	test.Equal(results[2].Value(), uint16(0xffff))
	test.Equal(results[3].Value(), byte(3))
	test.Equal(results[4].Value(), "aaa")
}

func TestFormat3(t *testing.T) {
	results, err := BinaryRead(
		bytes.NewBufferString("\x33\x22\x80\xff\xff\x03aaa"),
		NewInt16("ccc"),
		NewUint8("bbb"),
		NewUint16("ddd"),
		NewUint8("eee"),
		NewBuffer("value", "eee"),
	)
	if err != nil {
		t.Fatal(err)
	}
	test := assert.New(t)
	test.Equal(results[0].LittleEndian().AsInt16(), int16(0x2233))
	test.Equal(results[1].GetBytes(), []byte("\x80"))
	test.Equal(results[1].GetBytes(), []byte("\x80"))

	ret := results[0].BigEndian().AsInt16()
	test.Equal(ret, int16(0x3322))
	test.Equal(results[2].Value(), uint16(0xffff))
	test.Equal(results[3].Value(), byte(3))
	test.Equal(results[4].Value(), "aaa")
}

func TestFormat2(t *testing.T) {
	va := func(i any) int64 {
		return int64(utils.InterfaceToInt(uint8(199)))
	}
	origin := int8(va(1))
	spew.Dump(origin)
}
