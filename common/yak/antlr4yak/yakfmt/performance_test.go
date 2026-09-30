package yakfmt

import (
	"fmt"
	"strings"
	"testing"
)

// Reuse the same deterministic adversarial inputs in round-trip validation and
// benchmarks. Sizes/counts identify input shape, not a promise of latency.
func stressSources() map[string]string {
	sources := make(map[string]string)
	for _, count := range []int{100, 1000, 10000} {
		params := make([]string, count)
		for i := range params {
			params[i] = fmt.Sprintf("argument_%d map[string][]int", i)
		}
		sources[fmt.Sprintf("params_%d", count)] = "func(" + strings.Join(params, ",") + ") (int,error){return 1,nil}"
		sources[fmt.Sprintf("call_%d", count)] = "result=f(" + strings.Repeat("1,", count) + "0)"
		sources[fmt.Sprintf("list_%d", count)] = "result=[" + strings.Repeat("1,", count) + "0]"
		sources[fmt.Sprintf("chain_%d", count)] = "result=1" + strings.Repeat(" + 1", count)
	}
	for _, depth := range []int{8, 64, 256} {
		sources[fmt.Sprintf("depth_%d", depth)] = "result=" + strings.Repeat("func(){return ", depth) + "1" + strings.Repeat("}", depth)
	}
	for _, size := range []int{65536, 1048576} {
		payload := strings.Repeat("x", size)
		sources[fmt.Sprintf("raw_%dB", size)] = "result=`" + payload + "`"
		sources[fmt.Sprintf("comment_%dB", size)] = "// " + payload + "\nresult=1"
		sources[fmt.Sprintf("heredoc_%dB", size)] = "result=<<<TAG\n" + payload + "\nTAG"
		sources[fmt.Sprintf("template_%dB", size)] = "result=f\"" + payload + "${1+2}\""
	}
	return sources
}

func TestFormatStressMaterials(t *testing.T) {
	for name, source := range stressSources() {
		t.Run(name, func(t *testing.T) {
			// For megabyte character-token payloads, compare exact output
			// bytes directly instead of allocating several million-node
			// reference parse trees. Smaller instances check tree structure.
			if strings.HasSuffix(name, "1048576B") && (strings.HasPrefix(name, "heredoc_") || strings.HasPrefix(name, "template_")) {
				want := strings.Replace(source, "result=", "result = ", 1) + "\n"
				want = strings.Replace(want, "${1+2}", "${1 +\n    2}", 1)
				got, err := Format(source)
				if err != nil || got != want {
					t.Fatalf("changed large payload: %v, got %d bytes, want %d: %q", err, len(got), len(want), got[max(0, len(got)-48):])
				}
				return
			}
			assertRoundTrip(t, source, nil)
			if strings.HasPrefix(name, "params_") || strings.HasPrefix(name, "call_") || strings.HasPrefix(name, "list_") || strings.HasPrefix(name, "chain_") {
				got, err := Format(source)
				if err != nil {
					t.Fatal(err)
				}
				assertWidth(t, got)
			}
		})
	}
}

func invalidStressSources() map[string]string {
	sources := make(map[string]string)
	for _, size := range []int{65536, 524288} {
		valid := strings.Repeat("a=1\n", size/4)
		sources[fmt.Sprintf("early_%dB", size)] = "@\n" + valid
		sources[fmt.Sprintf("late_%dB", size)] = valid + "a=["
		sources[fmt.Sprintf("late_balanced_%dB", size)] = valid + "a="
		sources[fmt.Sprintf("late_block_%dB", size)] = valid + "func(){return ["
	}
	sources["unterminated_1MiB"] = "a=\"" + strings.Repeat("x", 1048576)
	sources["unclosed_depth_256"] = "a=" + strings.Repeat("(", 256)
	return sources
}

func TestFormatInvalidStress(t *testing.T) {
	for name, source := range invalidStressSources() {
		t.Run(name, func(t *testing.T) {
			if got, err := Format(source); err == nil || got != "" {
				t.Fatalf("invalid source produced output: %q, %v", excerpt(got), err)
			}
			if got, err := Format("a=1"); err != nil || got != "a = 1\n" {
				t.Fatal("failure contaminated next invocation", got, err)
			}
		})
	}
}

func BenchmarkFormatStress(b *testing.B) {
	for name, source := range stressSources() {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for i := 0; i < b.N; i++ {
				if got, err := Format(source); err != nil || got == "" {
					b.Fatal("format failed", err)
				}
			}
		})
	}
}

func BenchmarkFormatInvalid(b *testing.B) {
	for name, source := range invalidStressSources() {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			// Early errors deliberately do not report input MB/s: most bytes
			// are never parsed, so that rate would be misleading.
			for i := 0; i < b.N; i++ {
				if got, err := Format(source); err == nil || got != "" {
					b.Fatal("invalid source escaped", err)
				}
			}
		})
	}
}

func BenchmarkFormatParallel(b *testing.B) {
	source := strings.Repeat("f=func(a){if(a>0){return a+1};return 0}\n", 200)
	// Prime the shared ANTLR automata outside the measurement.
	if _, err := Format(source); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if got, err := Format(source); err != nil || got == "" {
				b.Error("format failed", err)
				return
			}
		}
	})
}
