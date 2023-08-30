package pcapx

import (
	"testing"
)

func BenchmarkPacketBuilder_ICMP(b *testing.B) {
	// do arp cache
	PacketBuilder(
		WithIPv4_SrcIP("1.1.1.1"),
		WithIPv4_DstIP("2.2.2.2"),
		WithICMP_Id(111),
	)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := PacketBuilder(
			//WithEthernet_DstMac("00:00:00:00:00:00"),
			//WithEthernet_SrcMac("00:00:00:00:00:00"),
			WithIPv4_SrcIP("1.1.1.1"),
			WithIPv4_DstIP("2.2.2.2"),
			WithICMP_Id(111),
		)
		if err != nil {
			b.Error(err)
			return
		}
	}
}
