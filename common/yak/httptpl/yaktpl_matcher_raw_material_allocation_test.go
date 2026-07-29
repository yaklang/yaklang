package httptpl

import (
	"fmt"
	"testing"
)

const legacyRawFallbackScope = "phase75-legacy-raw-fallback"

func executeLegacyRawScopeMatcher(matcher *YakMatcher, packet []byte) (bool, error) {
	legacy := *matcher
	// Unknown scopes have always fallen through to raw packet material, but do
	// not take the explicit raw-scope fast path. This retains the old
	// hash/cache implementation as an in-binary semantic and benchmark oracle.
	legacy.Scope = legacyRawFallbackScope
	return legacy.ExecuteRaw(packet, nil)
}

func TestRawScopeMaterialFastPathPreservesResults(t *testing.T) {
	tests := []struct {
		name    string
		packet  string
		matcher *YakMatcher
	}{
		{
			name:   "default word",
			packet: "prefix target suffix",
			matcher: &YakMatcher{
				MatcherType: MATCHER_TYPE_WORD,
				Group:       []string{"target"},
			},
		},
		{
			name:   "explicit raw suffix",
			packet: "application/problem+json",
			matcher: &YakMatcher{
				MatcherType: MATCHER_TYPE_SUFFIX,
				Scope:       SCOPE_RAW,
				Group:       []string{"+json"},
			},
		},
		{
			name:   "raw regexp",
			packet: "status=200",
			matcher: &YakMatcher{
				MatcherType: MATCHER_TYPE_REGEXP,
				Scope:       SCOPE_RAW,
				Group:       []string{`status=\d+`},
			},
		},
		{
			name:   "raw glob",
			packet: "api.performance.test",
			matcher: &YakMatcher{
				MatcherType: MATCHER_TYPE_GLOB,
				Scope:       SCOPE_RAW,
				Group:       []string{"*.performance.test"},
			},
		},
		{
			name:   "raw MIME",
			packet: "application/json",
			matcher: &YakMatcher{
				MatcherType: MATCHER_TYPE_MIME,
				Scope:       SCOPE_RAW,
				Group:       []string{"application/*"},
			},
		},
		{
			name:   "and miss",
			packet: "alpha beta",
			matcher: &YakMatcher{
				MatcherType: MATCHER_TYPE_WORD,
				Scope:       SCOPE_RAW,
				Condition:   "and",
				Group:       []string{"alpha", "gamma"},
			},
		},
		{
			name:   "negative",
			packet: "alpha beta",
			matcher: &YakMatcher{
				MatcherType: MATCHER_TYPE_WORD,
				Scope:       SCOPE_RAW,
				Negative:    true,
				Group:       []string{"gamma"},
			},
		},
		{
			name:   "hex group",
			packet: "alpha beta",
			matcher: &YakMatcher{
				MatcherType:   MATCHER_TYPE_WORD,
				Scope:         SCOPE_RAW,
				Group:         []string{"62657461"},
				GroupEncoding: GROUP_ENCODING_HEX,
			},
		},
		{
			name:   "binary matcher forces hex",
			packet: "alpha beta",
			matcher: &YakMatcher{
				MatcherType: MATCHER_TYPE_BIN,
				Scope:       SCOPE_RAW,
				Group:       []string{"62657461"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.matcher.PrecompileStaticGlobRules()
			packet := []byte(test.packet)
			want, wantErr := executeLegacyRawScopeMatcher(test.matcher, packet)
			got, gotErr := test.matcher.ExecuteRaw(packet, nil)
			if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
				t.Fatalf("error differs: got %v, want %v", gotErr, wantErr)
			}
			if got != want {
				t.Fatalf("result differs: got %v, want %v", got, want)
			}
		})
	}
}

func TestRawScopeMaterialFastPathReadsCurrentPacket(t *testing.T) {
	matcher := &YakMatcher{
		MatcherType: MATCHER_TYPE_WORD,
		Scope:       SCOPE_RAW,
		Group:       []string{"first"},
	}
	packet := []byte("first")
	got, err := matcher.ExecuteRaw(packet, nil)
	if err != nil || !got {
		t.Fatalf("initial match=%v error=%v", got, err)
	}
	copy(packet, "other")
	got, err = matcher.ExecuteRaw(packet, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("raw matcher retained material from the previous call")
	}
}

func FuzzRawScopeMaterialFastPathPreservesWordMatcher(f *testing.F) {
	for _, seed := range []struct {
		packet  string
		pattern string
	}{
		{packet: "prefix target suffix", pattern: "target"},
		{packet: "", pattern: ""},
		{packet: "你好，性能", pattern: "性能"},
		{packet: "\x00\xffraw", pattern: "\xff"},
	} {
		f.Add(seed.packet, seed.pattern)
	}

	f.Fuzz(func(t *testing.T, packet, pattern string) {
		if len(packet) > 4096 || len(pattern) > 256 {
			t.Skip()
		}
		matcher := &YakMatcher{
			MatcherType: MATCHER_TYPE_WORD,
			Scope:       SCOPE_RAW,
			Group:       []string{pattern},
		}
		raw := []byte(packet)
		want, wantErr := executeLegacyRawScopeMatcher(matcher, raw)
		got, gotErr := matcher.ExecuteRaw(raw, nil)
		if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
			t.Fatalf("error differs: got %v, want %v", gotErr, wantErr)
		}
		if got != want {
			t.Fatalf("result differs for packet %q and pattern %q: got %v, want %v", packet, pattern, got, want)
		}
	})
}

func BenchmarkRawScopeMaterialFastPath(b *testing.B) {
	packet := []byte("text/html")
	for _, benchmark := range []struct {
		name   string
		legacy bool
	}{
		{name: "hash_and_cache", legacy: true},
		{name: "borrow_for_call"},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			matcher := &YakMatcher{
				MatcherType: MATCHER_TYPE_WORD,
				Scope:       SCOPE_RAW,
				Group:       []string{"not-present"},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var (
					got bool
					err error
				)
				if benchmark.legacy {
					got, err = executeLegacyRawScopeMatcher(matcher, packet)
				} else {
					got, err = matcher.ExecuteRaw(packet, nil)
				}
				if err != nil || got {
					b.Fatalf("unexpected result=%v error=%v", got, err)
				}
			}
		})
	}
}
