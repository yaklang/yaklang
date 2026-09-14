package pcaputil

import "testing"

func BenchmarkOpenPcapFileStreaming(b *testing.B) {
	const packets = 4096
	name := benchmarkCaptureFile(b, packets)
	b.SetBytes(packets * 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		var received int
		err := OpenPcapFile(name, WithTCPReassemblyStream(64<<10), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { received += len(f.Payload) }))
		if err != nil {
			b.Fatal(err)
		}
		if received != packets*1024 {
			b.Fatalf("received %d bytes", received)
		}
	}
}
