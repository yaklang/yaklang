package stream_parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
	"testing"
)

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

func BenchmarkNativeFieldTree(b *testing.B) {
	wire := memcachedFieldsTestFixtures()["stats-response"]
	fields, info, err := decodeMemcachedFields(wire, "stats-response")
	if err != nil {
		b.Fatal(err)
	}
	var doc yaml.MapSlice
	if err := yaml.Unmarshal([]byte("endian: big\nunit: byte\nPackage:\n  Message: {}\n"), &doc); err != nil {
		b.Fatal(err)
	}
	root, err := base.NewNodeTree(doc)
	if err != nil {
		b.Fatal(err)
	}
	n := root.Children[0].Children[0]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := buildExactByteFieldTree(n, fields, info, 0, uint64(len(wire)*8), "memcached-fields", "big"); err != nil {
			b.Fatal(err)
		}
	}
}
