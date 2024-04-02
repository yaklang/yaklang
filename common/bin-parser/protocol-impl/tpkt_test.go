package protocol_impl

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestTPKT(t *testing.T) {
	rawDataHex := "030000221de000004e5500c108302d304142434430c208302d305a59585731c0010a"
	data, err := codec.DecodeHex(rawDataHex)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := ParseTpkt(data)
	assert.Equal(t, uint8(3), packet.Version)
	assert.Equal(t, uint8(0), packet.Reserved)
	assert.Equal(t, "1de000004e5500c108302d304142434430c208302d305a59585731c0010a", codec.EncodeToHex(packet.TPDU))
	res, err := packet.Marshal()
	assert.Equal(t, rawDataHex, codec.EncodeToHex(res))
}
