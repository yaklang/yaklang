package httptpl

import (
	"fmt"
	"testing"
)

var defaultMITMMIMEGroups = []string{
	"image/*",
	"audio/*",
	"video/*",
	"application/ogg",
	"application/pdf",
	"application/msword",
	"application/x-ppt",
	"video/avi",
	"application/x-ico",
	"*zip",
}

func newStaticMIMEMatcher(precompile bool, groups ...string) *YakMatcher {
	matcher := &YakMatcher{
		MatcherType: MATCHER_TYPE_MIME,
		Group:       append([]string(nil), groups...),
	}
	if precompile {
		matcher.PrecompileStaticGlobRules()
	}
	return matcher
}

func TestPrecompileStaticMIMEGlobRulesPreservesResults(t *testing.T) {
	tests := []struct {
		name   string
		rule   string
		target string
	}{
		{name: "wildcard subtype matches", rule: "image/*", target: "image/png"},
		{name: "wildcard subtype misses", rule: "image/*", target: "text/html"},
		{name: "wildcard type matches", rule: "*/json", target: "application/json"},
		{name: "two wildcard components", rule: "app*/*json", target: "application/problem+json"},
		{name: "exact MIME", rule: "application/pdf", target: "application/pdf"},
		{name: "exact MIME case sensitive", rule: "APPLICATION/PDF", target: "application/pdf"},
		{name: "bare wildcard against type", rule: "*zip", target: "application/zip"},
		{name: "bare wildcard against subtype", rule: "*zip", target: "application/x-zip"},
		{name: "bare exact against type", rule: "application", target: "application/json"},
		{name: "bare exact against subtype", rule: "json", target: "application/json"},
		{name: "bare contains ignores case", rule: "JSON", target: "problem+json"},
		{name: "rule-only slash misses", rule: "image/*", target: "png"},
		{name: "invalid wildcard", rule: "[*", target: "anything"},
		{name: "invalid component wildcard", rule: "image/[*", target: "image/png"},
		{name: "empty", rule: "", target: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := newStaticMIMEMatcher(false, test.rule)
			precompiled := newStaticMIMEMatcher(true, test.rule)
			want, wantErr := legacy.ExecuteRaw([]byte(test.target), nil)
			got, gotErr := precompiled.ExecuteRaw([]byte(test.target), nil)
			if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
				t.Fatalf("error differs: got %v, want %v", gotErr, wantErr)
			}
			if got != want {
				t.Fatalf("result differs: got %v, want %v", got, want)
			}
		})
	}
}

func TestPrecompiledMIMEMatcherFallsBackForMutatedGroup(t *testing.T) {
	matcher := newStaticMIMEMatcher(true, "image/*")
	matcher.Group = []string{"application/*"}

	got, err := matcher.ExecuteRaw([]byte("application/json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("matcher did not compile a MIME pattern added after preparation")
	}
}

func TestPrecompiledMIMEMatcherConcurrentReads(t *testing.T) {
	matcher := newStaticMIMEMatcher(true, defaultMITMMIMEGroups...)
	const goroutines = 16
	const iterations = 100
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				got, err := matcher.ExecuteRaw([]byte("text/html"), nil)
				if err != nil {
					errs <- err
					return
				}
				if got {
					errs <- fmt.Errorf("unexpected match")
					return
				}
			}
			errs <- nil
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func FuzzPrecompileStaticMIMEGlobRulesPreservesResults(f *testing.F) {
	for _, seed := range []struct {
		rule   string
		target string
	}{
		{rule: "image/*", target: "image/png"},
		{rule: "app*/*json", target: "application/problem+json"},
		{rule: "*zip", target: "application/x-zip"},
		{rule: "JSON", target: "problem+json"},
		{rule: "[*", target: "anything"},
	} {
		f.Add(seed.rule, seed.target)
	}

	f.Fuzz(func(t *testing.T, rule, target string) {
		if len(rule) > 256 || len(target) > 1024 {
			t.Skip()
		}

		legacy := newStaticMIMEMatcher(false, rule)
		precompiled := newStaticMIMEMatcher(true, rule)
		precompiled.PrecompileStaticGlobRules()

		want, wantErr := legacy.ExecuteRaw([]byte(target), nil)
		got, gotErr := precompiled.ExecuteRaw([]byte(target), nil)
		if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
			t.Fatalf("error differs for rule %q and target %q: got %v, want %v", rule, target, gotErr, wantErr)
		}
		if got != want {
			t.Fatalf("result differs for rule %q and target %q: got %v, want %v", rule, target, got, want)
		}
	})
}

func BenchmarkStaticMIMEMatcher(b *testing.B) {
	subject := []byte("text/html")
	for _, benchmark := range []struct {
		name       string
		precompile bool
	}{
		{name: "compile_each_match"},
		{name: "precompiled", precompile: true},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			matcher := newStaticMIMEMatcher(benchmark.precompile, defaultMITMMIMEGroups...)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, err := matcher.ExecuteRaw(subject, nil)
				if err != nil || got {
					b.Fatalf("unexpected result=%v error=%v", got, err)
				}
			}
		})
	}
}
