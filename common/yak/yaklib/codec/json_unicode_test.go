package codec

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestJsonUnicodeDecode(t *testing.T) {
	var a = JsonUnicodeEncode("你好ab")
	spew.Dump(a)
	println(a)
	var result = JsonUnicodeDecode(a)
	if result != "你好ab" {
		panic("unicode decode failed")
	}
}
