package bin_parser

import (
	"bytes"
	"encoding/hex"

	"testing"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestDNSAAAAFromUnmodifiedEthernetCapture(t *testing.T) {
	// L1: ntop/nDPI 4cae778e7e8f846b34f11d4f8392504cdebd3db8,
	// tests/cfgs/default/pcap/gnutella.pcap, frame 21. This is incidental mDNS
	// traffic inside the existing unchanged Gnutella capture, not a new capture
	// or protocol count. Its AAAA and following A RR must both stay aligned.
	wire, err := trafficfixture.ReadFile("testdata/protocol-corpus/captures/ndpi/ndpi-gnutella.pcap")
	require.NoError(t, err)
	r, err := pcapgo.NewReader(bytes.NewReader(wire))
	require.NoError(t, err)
	var frame []byte
	for i := 0; i < 21; i++ {
		frame, _, err = r.ReadPacketData()
		require.NoError(t, err)
	}
	dns := mustChild(t, parseEthernet(t, frame), "IP", "UDP", "MDNS")
	answers := mustChild(t, dns, "Answers").Children()
	require.Len(t, answers, 2)
	require.Equal(t, uint64(28), uintVal(t, answers[0].Child("Type")))
	want, err := hex.DecodeString("fe80000000000000c50d519f96a4e108")
	require.NoError(t, err)
	require.Equal(t, want, mustChild(t, answers[0], "DNSAAAA", "Address").Value)
	require.Equal(t, []byte{10, 0, 2, 15}, mustChild(t, answers[1], "DNSA", "Address").Value)
}
