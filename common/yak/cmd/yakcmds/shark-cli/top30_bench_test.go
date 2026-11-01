package sharkcli

import "testing"

var top30RowSink packetSummary

func BenchmarkTop30ConversationProjection(b *testing.B) {
	packets, s := top30Replay(b, top30Cases()[8])
	p := packets[len(packets)-1]
	row := summarize(p)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		top30RowSink = s.streams.projectConversation(row, p.streamID)
	}
}
