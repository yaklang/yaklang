package stream_parser

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestAnyToBytes(t *testing.T) {
	v := 0xaced
	res, _ := ConvertToBytes(v, 16, "big")
	assert.Equal(t, "aced", codec.EncodeToHex(res))

	res, _ = ConvertToBytes(v, 16, "little")
	assert.Equal(t, "edac", codec.EncodeToHex(res))
}
