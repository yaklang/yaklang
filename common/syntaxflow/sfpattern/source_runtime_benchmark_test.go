package sfpattern

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// BenchmarkSourceScanWorkerPipeline keeps compilation outside the timer and
// measures a fresh snapshot per iteration. Run the same fixture before/after
// moving path selection into the file workers; no filesystem cache is involved.
func BenchmarkSourceScanWorkerPipeline(b *testing.B) {
	for _, scenario := range []string{"shared-path", "mixed-paths", "mostly-excluded"} {
		b.Run(scenario, func(b *testing.B) {
			const patterns, filesCount, groups = 128, 4096, 16
			bodies := make([]string, 0, patterns)
			for i := 0; i < patterns; i++ {
				ext := fmt.Sprintf("g%02d", i%groups)
				if scenario == "shared-path" {
					ext = "txt"
				}
				bodies = append(bodies, fmt.Sprintf("${*.%s}.pattern_regex(/needle%03d/) as $hit", ext, i))
			}
			plan, err := NewSourceScanPlan(sourceFrames(b, bodies...))
			require.NoError(b, err)
			b.Cleanup(func() { require.NoError(b, plan.Close()) })
			files := make(map[string]string, filesCount)
			body := strings.Repeat("ordinary source text without a matching keyword\n", 32)
			expectedFiles, expectedHits := 0, 0
			var expectedBytes int64
			for i := 0; i < filesCount; i++ {
				ext := fmt.Sprintf("g%02d", i%groups)
				if scenario == "shared-path" {
					ext = "txt"
				}
				eligible := scenario != "mostly-excluded" || i%64 == 0
				if !eligible {
					ext = "skip"
				}
				content := body
				if i%64 == 0 {
					content += fmt.Sprintf("needle%03d", i%patterns)
					if eligible {
						expectedHits++
					}
				}
				files[fmt.Sprintf("src/%05d.%s", i, ext)] = content
				if eligible {
					expectedFiles++
					expectedBytes += int64(len(content))
				}
			}
			b.ReportAllocs()
			b.SetBytes(expectedBytes)
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				rt := plan.Start(context.Background(), files)
				err := rt.Wait(context.Background())
				rt.Close()
				if err != nil {
					b.Fatal(err)
				}
				stats := rt.Stats()
				if stats.Files != expectedFiles || stats.Hits != expectedHits || stats.Bytes != expectedBytes {
					b.Fatalf("unexpected scan: %+v, want files=%d hits=%d bytes=%d", stats, expectedFiles, expectedHits, expectedBytes)
				}
			}
		})
	}
}
