//go:build gzip_embed

package embed

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

func BenchmarkCompressedAssetsFirstUse(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, err := gzip_embed.NewPreprocessingEmbed(&FSArchive, "resources.tar.gz")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := resources.Stat("."); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompressedAssetsWarmOpen(b *testing.B) {
	if _, err := FS.Stat("."); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := FS.Open("data/geo/city2coord.json")
		if err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}
