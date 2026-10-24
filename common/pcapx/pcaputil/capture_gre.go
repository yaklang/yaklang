package pcaputil

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

type captureGREDecoder struct{}

func (captureGREDecoder) Decode(w []byte, b gopacket.PacketBuilder) error {
	fail := func() error {
		b.SetTruncated()
		return fmt.Errorf("GRE optional header exceeds captured bytes or header budget")
	}
	if len(w) < 4 {
		return fail()
	}
	at := 4
	if w[0]&0xc0 != 0 {
		at += 4
	}
	if w[0]&0x20 != 0 {
		at += 4
	}
	if w[0]&0x10 != 0 {
		at += 4
	}
	if at > len(w) {
		return fail()
	}
	if w[0]&0x40 != 0 {
		for n := 0; ; n++ {
			if n >= 128 || at+4 > len(w) || at > 4096 {
				return fail()
			}
			size := int(w[at+3])
			end := w[at] == 0 && w[at+1] == 0 && size == 0
			at += 4 + size
			if at > len(w) {
				return fail()
			}
			if end {
				break
			}
		}
	}
	if w[1]&0x80 != 0 {
		at += 4
	}
	if at > len(w) || at > 4096 {
		return fail()
	}
	g := &layers.GRE{}
	if err := g.DecodeFromBytes(w, b); err != nil {
		return err
	}
	b.AddLayer(g)
	if g.Protocol == 0x22eb {
		if g.Version != 0 || g.RoutingPresent || !g.SeqPresent {
			return fmt.Errorf("ERSPAN III: unselected GRE profile")
		}
		return b.NextDecoder(captureERSPAN3Decoder{})
	}
	if g.Protocol == layers.EthernetTypeERSPAN && !g.SeqPresent {
		return b.NextDecoder(layers.LayerTypeEthernet) // ERSPAN Type I has no ERSPAN header.
	}
	return b.NextDecoder(g.NextLayerType())
}
