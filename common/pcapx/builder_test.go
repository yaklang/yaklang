package pcapx

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestPacketBuilder(t *testing.T) {
	raw, err := PacketBuilder(
		WithIPv4LayerBuilderConfigSrcIP("127.0.0.1"),
		WithIPv4LayerBuilderConfigDstIP("127.0.0.1"),
	)
	if err != nil {
		panic(err)
	}
	spew.Dump(raw)
}
