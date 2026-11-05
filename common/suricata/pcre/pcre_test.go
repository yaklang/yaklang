package pcre

import (
	"errors"
	"strings"
	"sync"
	"testing"

	pcre2lite "github.com/VillanCh/go-pcre2-lite"
)

func mustMatcher(t testing.TB, pattern string) *Matcher {
	t.Helper()
	parsed, err := ParsePCREStr(pattern)
	if err != nil {
		t.Fatal(err)
	}
	matcher, err := parsed.Matcher()
	if err != nil {
		t.Fatal(err)
	}
	return matcher
}

func TestDefaultMatchLimitsConfigurable(t *testing.T) {
	oldMatchLimit, oldDepthLimit := GetDefaultMatchLimits()
	t.Cleanup(func() {
		if err := SetDefaultMatchLimits(oldMatchLimit, oldDepthLimit); err != nil {
			t.Errorf("restore match limits: %v", err)
		}
	})

	if err := SetDefaultMatchLimits(0, 1); err == nil {
		t.Fatal("zero match limit must be rejected")
	}
	if err := SetDefaultMatchLimits(1, 0); err == nil {
		t.Fatal("zero depth limit must be rejected")
	}
	if err := SetDefaultMatchLimits(12_345, 678); err != nil {
		t.Fatal(err)
	}
	matchLimit, depthLimit := GetDefaultMatchLimits()
	if matchLimit != 12_345 || depthLimit != 678 {
		t.Fatalf("unexpected limits: match=%d depth=%d", matchLimit, depthLimit)
	}
}

func TestMatcherNormalPCRE(t *testing.T) {
	matches, err := mustMatcher(t, `/ab+c/i`).Match([]byte("xxABBC abbbc"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].Pos != 2 || matches[0].Len != 4 || matches[1].Pos != 7 || matches[1].Len != 5 {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}

func TestMatcherCatastrophicBacktrackingReturnsLimitError(t *testing.T) {
	matches, err := mustMatcher(t, `/(a+)+$/`).Match([]byte(strings.Repeat("a", 64) + "!"))
	if !errors.Is(err, pcre2lite.ErrMatchLimit) {
		t.Fatalf("expected match limit error, got matches=%v err=%v", matches, err)
	}
	if matches != nil {
		t.Fatalf("limit errors must not return partial matches: %v", matches)
	}
}

func TestMatcherDepthLimitError(t *testing.T) {
	oldMatchLimit, oldDepthLimit := GetDefaultMatchLimits()
	t.Cleanup(func() {
		if err := SetDefaultMatchLimits(oldMatchLimit, oldDepthLimit); err != nil {
			t.Errorf("restore match limits: %v", err)
		}
	})
	if err := SetDefaultMatchLimits(1_000_000, 1); err != nil {
		t.Fatal(err)
	}

	matches, err := mustMatcher(t, `/a/`).Match([]byte("a"))
	if !errors.Is(err, pcre2lite.ErrDepthLimit) {
		t.Fatalf("expected depth limit error, got matches=%v err=%v", matches, err)
	}
	if matches != nil {
		t.Fatalf("depth limit errors must not return partial matches: %v", matches)
	}
}

func TestMatcherFindNextErrorPropagatesWithoutPartialResults(t *testing.T) {
	matches, err := mustMatcher(t, `/a|(b+)+$/`).Match([]byte("a" + strings.Repeat("b", 64) + "!"))
	if !errors.Is(err, pcre2lite.ErrMatchLimit) {
		t.Fatalf("expected next-match limit error, got matches=%v err=%v", matches, err)
	}
	if matches != nil {
		t.Fatalf("next-match errors must discard partial results: %v", matches)
	}
}

func TestMatcherResultBudget(t *testing.T) {
	matches, err := mustMatcher(t, `/a/`).Match([]byte(strings.Repeat("a", DefaultResultLimit+1)))
	if !errors.Is(err, ErrResultLimit) {
		t.Fatalf("expected result budget error, got matches=%d err=%v", len(matches), err)
	}
	if matches != nil {
		t.Fatalf("result budget errors must discard partial results")
	}
}

func TestMatcherZeroLengthMatchesAdvance(t *testing.T) {
	matches, err := mustMatcher(t, `/a*/`).Match([]byte("bbb"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 4 {
		t.Fatalf("got %d matches, want 4: %v", len(matches), matches)
	}
	for i, match := range matches {
		if match.Pos != i || match.Len != 0 {
			t.Fatalf("match %d did not advance: %v", i, match)
		}
	}
}

func TestMatcherConcurrentUse(t *testing.T) {
	matcher := mustMatcher(t, `/ab+c/i`)
	const goroutines = 32
	const iterations = 100

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				matches, err := matcher.Match([]byte("abb ABBC abbbbc"))
				if err != nil {
					errs <- err
					return
				}
				if len(matches) != 2 {
					errs <- errors.New("unexpected concurrent match count")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func BenchmarkMatcherNormal(b *testing.B) {
	matcher := mustMatcher(b, `/[A-Z][a-z]+\d{2,4}/`)
	input := []byte("prefix Example2026 suffix")
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		if _, err := matcher.Match(input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMatcherMatchLimit(b *testing.B) {
	matcher := mustMatcher(b, `/(a+)+$/`)
	input := []byte(strings.Repeat("a", 64) + "!")
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		if _, err := matcher.Match(input); !errors.Is(err, pcre2lite.ErrMatchLimit) {
			b.Fatalf("expected match limit error, got %v", err)
		}
	}
}
