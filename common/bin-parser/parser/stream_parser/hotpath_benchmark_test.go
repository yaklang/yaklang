package stream_parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
	"strings"
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

// Each iteration owns its output config, avoiding a growing metadata replay
// journal in a diagnostic that repeatedly constructs fields on a prepared rule.
func BenchmarkNativeFieldTreeLong(b *testing.B) {
	wire := []byte(strings.Repeat("STAT metric 12345\r\n", 48) + "END\r\n")
	fields, info, err := decodeMemcachedFields(wire, "stats-response")
	if err != nil {
		b.Fatal(err)
	}
	parent := base.NewEmptyConfig()
	parent.SetItems(base.ConfigItem{Key: "endian", Value: "big"}, base.ConfigItem{Key: "parser", Value: "default"}, base.ConfigItem{Key: "unit", Value: "byte"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := &base.Node{Name: "Message", Cfg: base.NewConfig(parent)}
		if err := buildExactByteFieldTree(n, fields, info, 0, uint64(len(wire)*8), "memcached-fields", "big"); err != nil {
			b.Fatal(err)
		}
	}
}
