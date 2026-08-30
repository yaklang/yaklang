package crawler

import (
	stdregexp "regexp"
	"strings"
	"testing"
	"unicode/utf8"

	pcre2 "github.com/VillanCh/go-pcre2-lite"
	"github.com/stretchr/testify/require"
)

// TestMUSTPASS_AIJSPCREPrefilter locks the engine contract behind adaptive JS
// analysis. These rules intentionally need PCRE lookaround/backreferences;
// minirehs is a conservative existence gate and must never hide a true PCRE
// match.
func TestMUSTPASS_AIJSPCREPrefilter(t *testing.T) {
	t.Run("pcre-only-boundaries-and-byte-offsets", func(t *testing.T) {
		_, err := stdregexp.Compile(`(?<![A-Za-z0-9_$])fetch(?![A-Za-z0-9_$])\s*\(`)
		require.Error(t, err, "Go RE2 must not silently replace the PCRE lookaround contract")
		_, err = stdregexp.Compile(`(['"])[^'"]+\1`)
		require.Error(t, err, "Go RE2 must not silently replace paired-quote backreferences")

		pattern := getAIJSTriggerPatterns().pattern("request-sink-call")
		require.NotNil(t, pattern)
		unicodeSource := []byte("你;FETCH(route);FETCH(other);FETCH(last);FETCH(extra)")
		matches := pattern.findAllIndexes(unicodeSource, 3)
		require.Len(t, matches, 3)
		require.Equal(t, len([]byte("你;")), matches[0][0], "PCRE offsets must remain UTF-8 byte offsets")
		require.Equal(t, "FETCH(", string(unicodeSource[matches[0][0]:matches[0][1]]))

		invalidUTF := append([]byte{0xff, ';'}, []byte("fetch(route)")...)
		matches = pattern.findAllIndexes(invalidUTF, 1)
		require.Len(t, matches, 1)
		require.Equal(t, 2, matches[0][0], "byte-mode PCRE must tolerate malformed response bytes")
	})

	t.Run("minirehs-gates-admit-every-precise-positive", func(t *testing.T) {
		tests := []struct {
			name   string
			source string
		}{
			{"adaptive-request-call", `$fetch(endpoint)`},
			{"adaptive-bracket-request", `globalThis["fetch"](endpoint)`},
			{"adaptive-open", `xhr.open(method,endpoint)`},
			{"adaptive-new-channel", `new WebSocket(endpoint)`},
			{"adaptive-service-worker", `navigator.serviceWorker.register(workerURL)`},
			{"adaptive-dynamic-module", `import(loader(id))`},
			{"adaptive-encoding", `String.fromCharCode(47,97,112,105)`},
			{"adaptive-escaped-bytes", `const value="\x2f\x61\x70\x69"`},
			{"adaptive-route-config", `baseURL=routeRoot`},
			{"adaptive-chunk", `__webpack_require__.u(chunkID)`},
			{"absolute-url", `const u="https://example.test/a?x=1";`},
			{"fetch-literal", `fetch('/api/v1')`},
			{"open-literal", `xhr.open('GET','/api/v1')`},
			{"new-literal", `new URL('/api/v1')`},
			{"axios-literal", `axios.post('/api/v1')`},
			{"http-library-literal", `superagent.post('/api/v1')`},
			{"jquery-literal", "$\n .\tget('/api/v1')"},
			{"module-literal", `require('./route.js')`},
			{"assignment-literal", `endpoint='/api/v1'`},
			{"path", `const p='/api/v1'`},
			{"resource-suffix", `const p='assets/app.js'`},
			{"quoted-route", `const p='/api/v1'`},
			{"quoted-file", `const p='deep.js'`},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				data := []byte(test.source)
				patternSet := getAIJSAdaptivePatternSet()
				pattern := patternSet.pattern(test.name)
				require.NotNil(t, pattern)
				precise, matchErr := pattern.re.Match(data)
				require.NoError(t, matchErr)
				require.True(t, precise, "test corpus must be a direct PCRE positive")

				active := false
				for _, candidate := range patternSet.activePatterns(data) {
					if candidate.name == test.name {
						active = true
						break
					}
				}
				require.True(t, active, "minirehs gate produced a false negative for %s", test.name)
			})
		}
	})

	t.Run("paired-quotes-and-url-terminators", func(t *testing.T) {
		patternSet := getAIJSAdaptivePatternSet()
		fetchLiteral := patternSet.pattern("fetch-literal")
		require.NotNil(t, fetchLiteral)
		for _, source := range []string{
			`fetch('/api/v1')`,
			`fetch("/api/v1")`,
			"fetch(`/api/v1`)",
			`fetch('/api/it\'s')`,
			`fetch('` + strings.Repeat("a", 1000) + `')`,
		} {
			matched, matchErr := fetchLiteral.re.Match([]byte(source))
			require.NoError(t, matchErr)
			require.Truef(t, matched, "expected paired literal match: %s", source)
		}
		for _, source := range []string{
			`fetch("/api/v1')`,
			`fetch('` + strings.Repeat("a", 1001) + `')`,
		} {
			matched, matchErr := fetchLiteral.re.Match([]byte(source))
			require.NoError(t, matchErr)
			require.Falsef(t, matched, "mismatched/oversized literal must not match: %s", source)
		}

		hits := rawCandidateHitsBounded(`const u='https://x.test/a?x=1';next();`, 16)
		require.Contains(t, hits, "https://x.test/a?x=1")
		for _, hit := range hits {
			require.NotContains(t, hit, "'")
			require.NotContains(t, hit, ";")
			require.NotContains(t, hit, ")")
		}
		punctuatedURL := "https://x.test/a(b,c;d)?next=(one,two);mode=full"
		hits = rawCandidateHitsBounded(`const u="`+punctuatedURL+`";next();`, 16)
		require.Contains(t, hits, punctuatedURL,
			"RFC-valid comma, semicolon, and parentheses must not truncate an absolute URL")
		hits = rawCandidateHitsBounded(`const a='https://x.test/a';b='junk';`, 16)
		require.Contains(t, hits, "https://x.test/a")
		for _, hit := range hits {
			require.NotContains(t, hit, ";b=",
				"a quoted URL must stop at its first matching delimiter")
		}
		hits = rawCandidateHitsBounded("API_URL=https://x.test/env/path\n", 16)
		require.Contains(t, hits, "https://x.test/env/path",
			"an unquoted config value must retain legacy absolute-URL discovery")
		hits = rawCandidateHitsBounded("myhttps://x.test/not-a-scheme", 16)
		require.NotContains(t, hits, "https://x.test/not-a-scheme",
			"an absolute URL must not begin inside an identifier")
		quotedRoute := patternSet.pattern("quoted-route")
		require.NotNil(t, quotedRoute)
		matched, matchErr := quotedRoute.re.Match([]byte(`const noise='abcdef'`))
		require.NoError(t, matchErr)
		require.False(t, matched, "a quoted route must contain a real slash-separated boundary")
	})

	t.Run("segmented-windows-preserve-real-boundaries", func(t *testing.T) {
		prefix := strings.Repeat("x", 24*1024-17)
		completeURL := "https://boundary.example.test/" + strings.Repeat("segment/", 120) + "finish.json"
		source := prefix + `const u="` + completeURL + `";` + strings.Repeat("z", 24*1024)
		pattern := getAIJSAdaptivePatternSet().pattern("absolute-url")
		require.NotNil(t, pattern)
		indexes, matchErr := findPCREMatchesEvenly(pattern, []byte(source), 32)
		require.NoError(t, matchErr)
		require.Len(t, indexes, 1)
		require.Equal(t, completeURL, source[indexes[0][0]:indexes[0][1]],
			"an artificial region edge must neither truncate nor validate a partial URL")

		unicodeSource := "你你" + `fetch(endpoint)`
		blocks := extractAdaptiveURLLikeCandidatesBounded(unicodeSource, 1, 4, 4096)
		require.NotEmpty(t, blocks)
		for _, block := range blocks {
			require.True(t, utf8.ValidString(block), "a valid source must not acquire an invalid UTF-8 evidence edge")
		}
	})

	t.Run("engine-errors-fail-open-with-bounded-evidence", func(t *testing.T) {
		re, compileErr := pcre2.Compile(`^(a+)+$`, pcre2.CompileOptions{MatchLimit: 1, DepthLimit: 32})
		require.NoError(t, compileErr)
		t.Cleanup(func() { _ = re.Close() })
		pattern := &aiJSPattern{name: "adversarial", re: re, gates: []string{"a"}}
		set := &aiJSPatternSet{patterns: []*aiJSPattern{pattern}, alwaysOn: []int{0}}
		source := []byte(strings.Repeat("a", 4096) + "b")
		matched := set.matchedNames(source)
		require.True(t, matched[aiJSPCREEngineErrorSignal],
			"a PCRE resource error must not be treated as a clean no-match")
		_, matchErr := findPCREMatchesEvenly(pattern, source, 8)
		require.Error(t, matchErr)
		fallback := fallbackAIJSPatternWindows(pattern, source, 120)
		require.NotEmpty(t, fallback)
		for _, window := range fallback {
			require.LessOrEqual(t, window.winEnd-window.winStart, 4096,
				"engine-error fallback must remain bounded")
		}
	})

	t.Run("regex-literals-are-not-request-surfaces", func(t *testing.T) {
		for _, source := range []string{
			`const matcher=/fetch\(endpoint\)|\/api\/v1/gi;const value=1;`,
			`prefetch(endpoint)`,
			`myfetch(endpoint)`,
			`fetcher(endpoint)`,
			`notfetch(endpoint)`,
		} {
			assessment := assessAIJSTrigger(source, "https://example.test/app.js", "application/javascript")
			require.Lessf(t, assessment.score, 3, "identifier or regex-literal noise triggered: %s", source)
			for _, hit := range rawCandidateHitsBounded(source, 16) {
				require.NotContains(t, hit, "/api")
			}
		}
	})

	t.Run("every-trigger-has-bounded-source-evidence", func(t *testing.T) {
		for _, source := range []string{
			`$fetch(endpoint)`,
			"fetch(\n endpoint)",
			`globalThis["fetch"](endpoint)`,
			`globalThis["\u0066etch"](endpoint)`,
			`globalThis["f\u0065tch"](endpoint)`,
			`axios['post'](endpoint)`,
			`fetch(!enabled?fallback:route)`,
			`import(loader(id))`,
			"import(\n loader(id))",
			`require(loader(id))`,
		} {
			assessment := assessAIJSTrigger(source, "https://example.test/app.js", "application/javascript")
			require.GreaterOrEqualf(t, assessment.score, 3, "source did not trigger: %s", source)
			blocks := extractAdaptiveURLLikeCandidatesBounded(source, 120, 8, 4096)
			require.NotEmptyf(t, blocks, "trigger had no candidate evidence: %s", source)
			require.Contains(t, strings.Join(blocks, "\n"), source)
		}
	})

	t.Run("comment-call-noise-cannot-starve-live-tail-evidence", func(t *testing.T) {
		source := "/*" + strings.Repeat(` fetch(fakeEndpoint);`, 512) +
			` */fetch(liveTailEndpoint)`
		assessment := assessAIJSTrigger(source, "https://example.test/app.js", "application/javascript")
		require.GreaterOrEqual(t, assessment.score, 3)
		blocks := extractAdaptiveURLLikeCandidatesBounded(source, 120, 1, 4096)
		require.Len(t, blocks, 1)
		require.Contains(t, blocks[0], "fetch(liveTailEndpoint)")
		require.NotContains(t, blocks[0], "fetch(fakeEndpoint)")
	})

	t.Run("five-megabyte-comment-noise-is-compacted", func(t *testing.T) {
		const size = 5 * 1024 * 1024
		source := "/*" + strings.Repeat("x", size-4) + "*/fetch(endpoint)"
		normalized := normalizeAIJSTriggerCode(source)
		require.Less(t, len(normalized.code), 64)
		assessment := assessAIJSTrigger(source, "https://example.test/app.huge.js", "application/javascript")
		require.GreaterOrEqual(t, assessment.score, 3)

		neutral := []byte(strings.Repeat("x", size))
		require.LessOrEqual(t, len(getAIJSAdaptivePatternSet().activePatterns(neutral)), 2,
			"neutral input should only retain explicitly always-on path families")
	})
}
