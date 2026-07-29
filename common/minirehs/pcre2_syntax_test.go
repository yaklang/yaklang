package minirehs

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	pcre2 "github.com/VillanCh/go-pcre2-lite/regexp2"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// TestPCRE2FeatureMatrixMVS covers the major syntax families listed by the
// PCRE2 10.47 pattern/syntax specifications. Native PCRE2 is the route-B
// language-superset oracle; YakRegexpUtils is the minirehs behavioral oracle
// because expressions accepted by Go regexp intentionally retain RE2 priority.
func TestPCRE2FeatureMatrixMVS(t *testing.T) {
	type featureCase struct {
		feature string
		pattern string
		input   string
		want    bool
	}
	cases := []featureCase{
		// Anchors, match-start reset, quoting, newline and Unicode atoms.
		{"absolute-anchors", `\Afoo\z`, "foo", true},
		{"braced-octal", `\o{101}\x{42}\N{U+0043}`, "ABC", true},
		{"control-and-escape", `\cA\e`, "\x01\x1b", true},
		{"before-final-newline", `foo\Z`, "foo\n", true},
		{"start-of-match", `\Gfoo`, "foo", true},
		{"match-start-reset", `prefix\Kpayload`, "xxprefixpayloadyy", true},
		{"quoted-literal", `\Q[a-z]+(x)\E`, "xx[a-z]+(x)yy", true},
		{"quoted-routeb-metacharacters", `(?=foo)foo\Q$\v(abc)\1\Eend`, `foo$\v(abc)\1end`, true},
		{"unicode-newline-lf", `left\Rright`, "left\nright", true},
		{"unicode-newline-crlf", `left\Rright`, "left\r\nright", true},
		{"grapheme-combining", `^\X$`, "e\u0301", true},
		{"grapheme-zwj", `^\X$`, "😀\u200d😀", true},
		{"horizontal-space", `a\hb`, "a\tb", true},
		{"vertical-space", `a\vb`, "a\nb", true},
		{"vertical-space-after-lookahead", `(?=prefix)prefix\vbody`, "prefix\nbody", true},
		{"dollar-before-final-newline", `(?=prefix)prefix$`, "prefix\n", true},
		{"ucp-digit-after-lookahead", `(?=٣)\d+suffix`, "٣suffix", true},
		{"ucp-word-after-lookahead", `(?=é)\w+suffix`, "ésuffix", true},
		{"ucp-boundary-after-lookahead", `(?=é)é\b-suffix`, "é-suffix", true},
		{"ucp-class-conservative-bail", `(?=٣)[\d]+suffix`, "٣suffix", true},
		{"posix-class-conservative-bail", `(?=é)[[:alpha:]]+suffix`, "ésuffix", true},
		{"not-newline", `a\Nb`, "axb", true},
		{"unicode-script", `^\p{Script=Greek}+$`, "Ελλάδα", true},
		{"unicode-script-extension", `^\p{scx:Han}+$`, "漢字", true},
		{"unicode-negated-property", `^\P{Nd}+$`, "payload", true},
		{"posix-class", `[[:alpha:]]+`, "payload42", true},
		{"perl-extended-class", `(?[\p{Greek} & \p{L}])+`, "Ελλάδα", true},

		// Lookarounds, including PCRE2 alphabetic and non-atomic spellings.
		{"positive-lookahead", `foo(?=bar)`, "xxfoobaryy", true},
		{"negative-lookahead", `foo(?!bar)`, "xxfoobazyy", true},
		{"positive-lookbehind", `(?<=foo)bar`, "xxfoobaryy", true},
		{"negative-lookbehind", `(?<!foo)bar`, "xxquxbaryy", true},
		{"bounded-variable-lookbehind", `(?<=colou?r)token`, "colourtoken", true},
		{"alphabetic-pla", `(*pla:foo)foo`, "xxfooyy", true},
		{"alphabetic-nla", `(*nla:bar)foo`, "xxfooyy", true},
		{"alphabetic-plb", `foo(*plb:foo)bar`, "xxfoobaryy", true},
		{"alphabetic-nlb", `foo(*nlb:bar)bar`, "xxfoobaryy", true},
		{"non-atomic-lookahead", `(?*(a+))aab\1`, "aaaabaa", true},
		{"non-atomic-lookbehind", `(?<*(ab|c))\1`, "xxababyy", true},

		// Captures and every documented backreference spelling.
		{"numeric-backref", `(foo)\1`, "xxfoofooyy", true},
		{"numeric-g-backref", `(foo)\g1`, "xxfoofooyy", true},
		{"numeric-braced-backref", `(foo)\g{1}`, "xxfoofooyy", true},
		{"relative-backref", `(foo)(bar)\g{-1}`, "xxfoobarbar", true},
		{"named-angle-backref", `(?<x>foo)\k<x>`, "xxfoofooyy", true},
		{"named-quote-backref", `(?'x'foo)\k'x'`, "xxfoofooyy", true},
		{"named-brace-k-backref", `(?<x>foo)\k{x}`, "xxfoofooyy", true},
		{"named-brace-g-backref", `(?<x>foo)\g{x}`, "xxfoofooyy", true},
		{"python-backref", `(?P<x>foo)(?P=x)`, "xxfoofooyy", true},
		{"forward-backref", `\1(foo)`, "xxfooyy", false},
		{"duplicate-names", `(?J)(?<x>foo)|(?<x>bar)\k<x>`, "barbar", true},
		{"branch-reset", `(?|(a)|(b))\1`, "bb", true},

		// Atomicity and quantifier forms.
		{"atomic-group", `(?>foo|fo)obar`, "xxfoobarxx", false},
		{"alphabetic-atomic-group", `(*atomic:foo|fo)bar`, "foobar", true},
		{"possessive-star", `a*+ab`, "aaab", false},
		{"possessive-plus", `a++b`, "aaab", true},
		{"possessive-question", `a?+ab`, "aab", true},
		{"possessive-range", `a{2,4}+b`, "aaaab", true},
		{"ungreedy-option", `(?U)<.+>`, "<a><b>", true},
		{"extended-option", `(?x) foo \s+ bar`, "foo   bar", true},
		{"no-auto-capture", `(?n)(foo)(?<x>bar)\g{x}`, "foobarbar", true},
		{"ascii-digit-option", `(?aD)\d+`, "12345", true},
		{"option-reset", `(?i)foo(?-i)BAR`, "FooBAR", true},
		{"initial-match-limit", `(*LIMIT_MATCH=100000)payload`, "payload", true},
		{"initial-no-auto-possess", `(*NO_AUTO_POSSESS)payload`, "payload", true},
		{"initial-utf-ucp", `(*UTF)(*UCP)\w+`, "é", true},
		{"newline-cr", `(*CR)(?m)^payload$`, "head\rpayload\rtail", true},
		{"bsr-anycrlf", `(*BSR_ANYCRLF)left\Rright`, "left\r\nright", true},

		// Recursion and subroutine calls.
		{"whole-recursion", `^(.|(.)(?1)\2)$`, "abcba", true},
		{"numeric-subroutine", `(foo|bar)-(?1)`, "foo-bar", true},
		{"relative-subroutine", `(foo)(bar)(?-1)`, "foobarbar", true},
		{"named-subroutine-perl", `(?<word>foo|bar)-(?&word)`, "foo-bar", true},
		{"named-subroutine-python", `(?<word>foo|bar)-(?P>word)`, "foo-bar", true},
		{"oniguruma-subroutine", `(?<word>foo|bar)-\g<word>`, "foo-bar", true},
		{"recursive-balanced", `^(?<pn>\((?:[^()]++|(?&pn))*\))$`, "(a(b)c)", true},
		{"define-subroutine", `(?(DEFINE)(?<byte>[A-F0-9]{2}))^(?&byte):(?&byte)$`, "AF:09", true},

		// Conditional groups.
		{"capture-conditional-yes", `^(a)?(?(1)b|c)$`, "ab", true},
		{"capture-conditional-no", `^(a)?(?(1)b|c)$`, "c", true},
		{"named-conditional", `^(?<tag><)?foo(?(tag)>)$`, "<foo>", true},
		{"assertion-conditional", `(?(?=foo)foo|bar)`, "foo", true},
		{"version-conditional", `(?(VERSION>=10.47)yes|no)`, "yes", true},
		{"recursion-conditional", `^(?<pn>\((?:(?(R)&|x)|(?&pn))*\))$`, "(x(&)x)", true},

		// Backtracking control.
		{"accept", `foo(*ACCEPT)bar`, "foo", true},
		{"fail", `foo(*FAIL)|bar`, "bar", true},
		{"commit", `a(*COMMIT)b|ac`, "ac", false},
		{"prune", `a(*PRUNE)b|ac`, "ac", false},
		{"skip", `foo(*SKIP)(*FAIL)|bar`, "foobar", true},
		{"then", `(?:a(*THEN)b|ac)`, "ac", true},
		{"mark", `foo(*MARK:seen)bar`, "foobar", true},

		// PCRE2-special grouping forms.
		{"script-run", `(*script_run:\p{L}+)`, "hello", true},
		{"atomic-script-run", `(*atomic_script_run:\p{L}+)`, "Ελλάδα", true},
		{"inline-comment", `foo(?# ignored )bar`, "foobar", true},
		{"callout-number", `foo(?C1)bar`, "foobar", true},
		{"callout-string", `foo(?C"probe")bar`, "foobar", true},
		{"scan-substring", `([a-z])([a-z]++)(#+)(*scs:(2)(ab.))`, "yabc###", true},
		{"scan-substring-named", `(?<XX>[a-z]++)##(*scan_substring:('XX').*(..)$)\2`, "##abcd##abcd##cd##", true},

		// Realistic security-oriented combinations.
		{"repeated-header-backref", `(?i)^(?<h>x-[a-z-]+):\h*(\w+)\R\k<h>:\h*\2$`, "X-Token: secret\r\nX-Token: secret", true},
		{"quoted-secret", `(?<=["'])(?<secret>AKIA[A-Z0-9]{16})(?=["'])`, `"AKIAABCDEFGHIJKLMNOP"`, true},
		{"skip-comments-find-token", `(?s)/\*.*?\*/(*SKIP)(*F)|token=[A-Za-z0-9._-]+`, "/* token=fake */ token=real", true},
		{"recursive-jsonish", `(?<obj>\{(?:[^{}"]++|"(?:\\.|[^"])*"|(?&obj))*\})`, `{"a":{"token":"x"}}`, true},
		{"unicode-domain", `(?i)(?<![\p{L}\p{N}-])(?:[\p{L}\p{N}](?:[\p{L}\p{N}-]{0,61}[\p{L}\p{N}])?\.)+\p{L}{2,63}`, "访问例子.公司即可", true},
	}

	var compilable, routeB, gated, priorityDivergentCases, subjectChecks int
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%03d-%s", i, tc.feature), func(t *testing.T) {
			pcre, err := pcre2.Compile(tc.pattern, pcre2.None)
			if err != nil {
				t.Fatalf("PCRE2 rejected feature pattern %q: %v", tc.pattern, err)
			}
			pcreWant, err := pcre.MatchString(tc.input)
			if err != nil {
				t.Fatalf("PCRE2 oracle match: %v", err)
			}
			if pcreWant != tc.want {
				t.Fatalf("bad matrix expectation: PCRE2=%v want=%v pattern=%q input=%q", pcreWant, tc.want, tc.pattern, tc.input)
			}

			yak := regexp_utils.NewYakRegexpUtils(tc.pattern)
			if !yak.CanUse() {
				t.Fatalf("YakRegexpUtils rejected PCRE2 feature pattern %q", tc.pattern)
			}
			compilable++
			lits := extractRequiredLiteralsApprox(tc.pattern, 2)
			super, superOK := re2Superset(tc.pattern)
			superRE, superErr := regexp.Compile(super)
			if !superOK {
				superErr = fmt.Errorf("rewrite rejected")
			}
			if super, ok := re2Superset(tc.pattern); ok {
				// route-B is used only if the original expression is not RE2.
				// For that domain, every PCRE2-positive subject must remain in
				// the rewritten language or the prefilter could cause a miss.
				re, compileErr := regexp.Compile(super)
				if compileErr == nil {
					routeB++
					if len(lits) > 0 {
						gated++
					}
				}
				if _, re2Err := regexp.Compile(tc.pattern); re2Err != nil && pcreWant && compileErr == nil {
					if !re.MatchString(tc.input) {
						t.Fatalf("route-B SHRANK language: original=%q super=%q input=%q literals=%v",
							tc.pattern, super, tc.input, lits)
					}
				}
			}

			db, err := Compile([]Pattern{{ID: 0, Expr: tc.pattern}},
				WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatalf("minirehs compile: %v", err)
			}
			defer db.Close()
			sc, err := db.NewScratch()
			if err != nil {
				t.Fatalf("scratch: %v", err)
			}
			defer sc.Close()

			hadPriorityDivergence := false
			for sampleIdx, subject := range derivedPCRE2Subjects(tc.input) {
				subjectChecks++
				native, matchErr := pcre.MatchString(subject)
				if matchErr != nil {
					t.Fatalf("sample %d PCRE2 match: %v", sampleIdx, matchErr)
				}
				want, matchErr := yak.MatchString(subject)
				if matchErr != nil {
					t.Fatalf("sample %d Yak oracle match: %v", sampleIdx, matchErr)
				}
				if want != native {
					hadPriorityDivergence = true
				}
				if _, re2Err := regexp.Compile(tc.pattern); re2Err != nil &&
					native && superOK && superErr == nil && !superRE.MatchString(subject) {
					t.Fatalf("sample %d route-B SHRANK language: original=%q super=%q input=%q literals=%v",
						sampleIdx, tc.pattern, super, subject, lits)
				}

				got := false
				if err := db.Scan([]byte(subject), sc, func(Match) bool {
					got = true
					return false
				}); err != nil {
					t.Fatalf("sample %d scan: %v", sampleIdx, err)
				}
				if got != want {
					t.Fatalf("sample %d MVS mismatch got=%v oracle=%v pattern=%q input=%q literals=%v",
						sampleIdx, got, want, tc.pattern, subject, lits)
				}
			}
			if hadPriorityDivergence {
				priorityDivergentCases++
				t.Logf("documented RE2-priority semantic difference from native PCRE2: pattern=%q", tc.pattern)
			}
		})
	}

	// Compile the entire feature set together, then compare sequential, true
	// multi-lane batch, and concurrent scans. This catches cross-pattern slot,
	// dedup, scratch reuse, and C-kernel integration mistakes that per-pattern
	// checks cannot expose.
	patterns := make([]Pattern, len(cases))
	records := make([][]byte, len(cases))
	for i, tc := range cases {
		patterns[i] = Pattern{ID: PatternID(i), Expr: tc.pattern}
		records[i] = []byte(strings.Repeat("padding/", 64) + tc.input + strings.Repeat("/padding", 64))
	}
	all, err := Compile(patterns,
		WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile combined PCRE2 matrix: %v", err)
	}
	defer all.Close()

	seq := make([][]PatternID, len(records))
	seqScratch, err := all.NewScratch()
	if err != nil {
		t.Fatal(err)
	}
	defer seqScratch.Close()
	for record, data := range records {
		if err := all.Scan(data, seqScratch, func(m Match) bool {
			seq[record] = append(seq[record], m.ID)
			return true
		}); err != nil {
			t.Fatalf("sequential combined scan record=%d: %v", record, err)
		}
	}

	batched := make([][]PatternID, len(records))
	batchScratch, err := all.NewScratch()
	if err != nil {
		t.Fatal(err)
	}
	defer batchScratch.Close()
	if err := all.ScanBatch(records, batchScratch, func(record int, m Match) bool {
		batched[record] = append(batched[record], m.ID)
		return true
	}); err != nil {
		t.Fatalf("combined ScanBatch: %v", err)
	}
	if !reflect.DeepEqual(batched, seq) {
		t.Fatalf("combined PCRE2 ScanBatch differs from sequential")
	}

	concurrent := make([][]PatternID, len(records))
	var wg sync.WaitGroup
	errs := make(chan error, len(records))
	for record, data := range records {
		record, data := record, data
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc, scratchErr := all.NewScratch()
			if scratchErr != nil {
				errs <- scratchErr
				return
			}
			defer sc.Close()
			if scanErr := all.Scan(data, sc, func(m Match) bool {
				concurrent[record] = append(concurrent[record], m.ID)
				return true
			}); scanErr != nil {
				errs <- scanErr
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("combined concurrent scan: %v", err)
	}
	if !reflect.DeepEqual(concurrent, seq) {
		t.Fatalf("combined PCRE2 concurrent scans differ from sequential")
	}

	t.Logf("PCRE2 matrix: cases=%d subjects=%d compilable=%d routeB=%d gated=%d RE2-priority-divergent-cases=%d",
		len(cases), subjectChecks, compilable, routeB, gated, priorityDivergentCases)
}

func derivedPCRE2Subjects(seed string) []string {
	out := []string{
		seed,
		"prefix::" + seed,
		seed + "::suffix",
		"prefix::" + seed + "::suffix",
		seed + seed,
		"\x00" + seed + "\x00",
		"请求/😀/" + seed + "/响应/é",
		"GET /scan HTTP/1.1\r\nX-Probe: " + seed + "\r\n\r\n",
		`{"event":"` + seed + `","nested":{"ok":true}}`,
	}
	if len(seed) > 0 {
		out = append(out,
			seed[:len(seed)-1],
			seed[1:],
			seed[:len(seed)/2]+"#"+seed[len(seed)/2:],
		)
	}
	return out
}
