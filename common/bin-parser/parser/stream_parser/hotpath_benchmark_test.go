package stream_parser

import "testing"

// Native decoding alone excludes public Node construction and export. These
// fixed semantic fixtures are diagnostic inputs, not the 11-message corpus.
func BenchmarkNativeFieldDecoding(b *testing.B) {
	for _, profile := range []string{"stats-request", "stats-response", "binary-get-request"} {
		wire := memcachedFieldsTestFixtures()[profile]
		b.Run("Memcached/"+profile, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(wire)))
			for i := 0; i < b.N; i++ {
				if _, _, err := decodeMemcachedFields(wire, profile); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
