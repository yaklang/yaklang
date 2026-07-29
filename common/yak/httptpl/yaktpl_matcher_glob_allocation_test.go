package httptpl

import (
	"fmt"
	"testing"

	"github.com/gobwas/glob"
)

func newStaticGlobMatcher(precompile bool) *YakMatcher {
	matcher := &YakMatcher{
		MatcherType: MATCHER_TYPE_GLOB,
		Group: []string{
			"google.com",
			"*gstatic.com",
			"*bdstatic.com",
			"*google*.com",
		},
	}
	if precompile {
		matcher.PrecompileStaticGlobRules()
	}
	return matcher
}

func TestPrecompileStaticGlobRulesPreservesResults(t *testing.T) {
	subjects := []string{
		"google.com",
		"www.gstatic.com",
		"cdn.bdstatic.com",
		"mail.googleapis.com",
		"performance.example.test",
		"",
	}
	legacy := newStaticGlobMatcher(false)
	precompiled := newStaticGlobMatcher(true)
	for _, subject := range subjects {
		t.Run(subject, func(t *testing.T) {
			want, wantErr := legacy.ExecuteRaw([]byte(subject), nil)
			got, gotErr := precompiled.ExecuteRaw([]byte(subject), nil)
			if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
				t.Fatalf("error differs: got %v, want %v", gotErr, wantErr)
			}
			if got != want {
				t.Fatalf("result differs: got %v, want %v", got, want)
			}
		})
	}
}

func TestPrecompiledGlobMatcherFallsBackForMutatedGroup(t *testing.T) {
	matcher := newStaticGlobMatcher(true)
	matcher.Group = []string{"*.example.test"}

	got, err := matcher.ExecuteRaw([]byte("api.example.test"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("matcher did not compile a pattern added after preparation")
	}
}

func TestPrecompiledGlobMatcherConcurrentReads(t *testing.T) {
	matcher := newStaticGlobMatcher(true)
	const goroutines = 16
	const iterations = 100
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				got, err := matcher.ExecuteRaw([]byte("performance.example.test"), nil)
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

func FuzzPrecompileStaticGlobRulesPreservesResults(f *testing.F) {
	for _, seed := range []struct {
		pattern string
		subject string
	}{
		{pattern: "*.example.com", subject: "api.example.com"},
		{pattern: "api-?.example.{com,net}", subject: "api-1.example.net"},
		{pattern: "[a-z]*.test", subject: "performance.test"},
		{pattern: `literal\\*value`, subject: `literal\anythingvalue`},
	} {
		f.Add(seed.pattern, seed.subject)
	}

	f.Fuzz(func(t *testing.T, pattern, subject string) {
		if len(pattern) > 256 || len(subject) > 1024 {
			t.Skip()
		}
		if _, err := glob.Compile(pattern); err != nil {
			t.Skip()
		}

		legacy := &YakMatcher{MatcherType: MATCHER_TYPE_GLOB, Group: []string{pattern}}
		precompiled := &YakMatcher{MatcherType: MATCHER_TYPE_GLOB, Group: []string{pattern}}
		precompiled.PrecompileStaticGlobRules()

		want, wantErr := legacy.ExecuteRaw([]byte(subject), nil)
		got, gotErr := precompiled.ExecuteRaw([]byte(subject), nil)
		if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
			t.Fatalf("error differs: got %v, want %v", gotErr, wantErr)
		}
		if got != want {
			t.Fatalf("result differs for pattern %q and subject %q: got %v, want %v", pattern, subject, got, want)
		}
	})
}

func BenchmarkStaticGlobMatcher(b *testing.B) {
	subject := []byte("performance.example.test")
	for _, benchmark := range []struct {
		name       string
		precompile bool
	}{
		{name: "compile_each_match"},
		{name: "precompiled", precompile: true},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			matcher := newStaticGlobMatcher(benchmark.precompile)
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
