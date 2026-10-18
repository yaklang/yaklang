package minirehs

import (
	"bytes"
	"fmt"
	"regexp/syntax"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

// Keep input shape and reporting semantics explicit: an existence benchmark
// does different work from enumerating every match in a repetitive payload.
func BenchmarkRuntimeScenarios(b *testing.B) {
	for _, set := range []string{"GetRequest", "MITM", "Literal1000", "PCRE128"} {
		b.Run(set, func(b *testing.B) {
			var pats []Pattern
			switch set {
			case "GetRequest":
				pats = getRequestImagePatterns(b)
			case "MITM":
				pats, _ = compilableMITMPatterns(b)
			case "Literal1000":
				pats = genSyntheticPatterns(1000)
			case "PCRE128":
				for i := 0; i < 128; i++ {
					pats = append(pats, Pattern{ID: PatternID(i + 1), Expr: fmt.Sprintf(`common-prefix[0-9]{2}(?=token%03d)token%03d`, i, i)})
				}
			}
			for _, locate := range []bool{false, true} {
				b.Run(map[bool]string{false: "Exist", true: "Locate"}[locate], func(b *testing.B) {
					db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
					if err != nil {
						b.Fatal(err)
					}
					defer db.Close()
					for _, payload := range []struct {
						name string
						data []byte
					}{
						{"HTTP912", bytes.Repeat([]byte("HTTP/1.1 200 OK\r\nServer: nginx/1.24.0\r\nContent-Type: text/html\r\n\r\n<html> token=abc123 header-042: abcd;ef </html>\n"), 8)},
						{"Sparse16K", append(bytes.Repeat([]byte("zqxv "), 3276), []byte(" tok00042zdeadbeef common-prefix42token042 ")...)},
						{"DenseHTTP16K", bytes.Repeat([]byte("HTTP/1.1 200 OK\r\nServer: nginx\r\n\r\n"), 512)},
						{"Zero16K", make([]byte, 16<<10)},
					} {
						b.Run(payload.name, func(b *testing.B) {
							sc, err := db.NewScratch()
							if err != nil {
								b.Fatal(err)
							}
							defer sc.Close()
							matches, err := imageMatches(db, payload.data)
							if err != nil {
								b.Fatal(err)
							}
							var signature uint64 = 14695981039346656037
							for _, m := range matches {
								for _, v := range []uint64{uint64(m.ID), uint64(m.From), uint64(m.To)} {
									signature = (signature ^ v) * 1099511628211
								}
							}
							b.Logf("matches=%d signature=%016x bytes=%d", len(matches), signature, len(payload.data))
							if err := db.Scan(payload.data, sc, nil); err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(payload.data)))
							b.ReportAllocs()
							b.ResetTimer()
							b.ReportMetric(float64(len(matches)), "hits/op")
							for i := 0; i < b.N; i++ {
								if err := db.Scan(payload.data, sc, nil); err != nil {
									b.Fatal(err)
								}
							}
						})
					}
				})
			}
		})
	}
}

type runtimeVerifierCount struct {
	verifier
	calls, bytes int64
}

func (v *runtimeVerifierCount) findAll(data []byte) [][2]int {
	atomic.AddInt64(&v.calls, 1)
	atomic.AddInt64(&v.bytes, int64(len(data)))
	return v.verifier.findAll(data)
}

// This diagnostic intentionally retains independent per-verifier subjects;
// it measures candidate selection and bytes verified, not copy performance.
func TestRuntimeGetRequestCandidates(t *testing.T) {
	requireDiag(t)
	db, _ := compileImageTest(t, getRequestImagePatterns(t), false)
	m := db.(*database).imageMVS()
	counts := make(map[int]*runtimeVerifierCount)
	for i, p := range m.all {
		if !p.v.exact() {
			counts[i] = &runtimeVerifierCount{verifier: p.v}
			p.v = counts[i]
		}
	}
	input := bytes.Repeat([]byte("HTTP/1.1 200 OK\r\nServer: nginx/1.24.0\r\nContent-Type: text/html\r\n\r\n<html> token=abc123 header-042: abcd;ef </html>\n"), 8)
	matches, err := imageMatches(db, input)
	if err != nil {
		t.Fatal(err)
	}
	lower := asciiLowerString(string(input))
	var triggered []int
	var calls, verifiedBytes, removable int64
	for idx, count := range counts {
		calls += count.calls
		verifiedBytes += count.bytes
		if count.calls == 0 {
			continue
		}
		triggered = append(triggered, idx)
		if super, _, ok := re2SupersetEx(m.all[idx].expr); ok {
			if parsed, err := syntax.Parse(super, syntax.Perl); err == nil {
				for _, factor := range extractRequiredLiteralFactors(parsed.Simplify(), 2) {
					present := false
					for _, lit := range factor {
						present = present || strings.Contains(lower, lit)
					}
					if !present {
						removable += count.calls
						break
					}
				}
			}
		}
	}
	sort.Slice(triggered, func(i, j int) bool {
		if counts[triggered[i]].bytes != counts[triggered[j]].bytes {
			return counts[triggered[i]].bytes > counts[triggered[j]].bytes
		}
		return triggered[i] < triggered[j]
	})
	t.Logf("patterns=%d PCRE2=%d matched=%d PCRE2calls=%d verifiedBytes=%d extraNecessaryFactorsCouldReject=%d", len(m.all), len(counts), len(matches), calls, verifiedBytes, removable)
	for i, idx := range triggered {
		if i == 12 {
			break
		}
		t.Logf("idx=%d calls=%d bytes=%d gate=%v NFA=%v literals=%q expr=%q", idx, counts[idx].calls, counts[idx].bytes, m.gate[idx], m.nfas[idx] != nil, m.all[idx].literals, m.all[idx].expr)
	}
}

func TestRuntimeCompileWork(t *testing.T) {
	requireDiag(t)
	parses, nfaAttempts := 0, 0
	imageParseHook = func() { parses++ }
	imageNFAHook = func() { nfaAttempts++ }
	defer func() { imageParseHook, imageNFAHook = nil, nil }()
	db, err := Compile(getRequestImagePatterns(t), WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := db.(*database).imageMVS()
	var positions, transitionWords, alphabetWords, exceptions int
	for _, n := range m.nfas {
		if n != nil {
			positions += n.npos
			transitionWords += n.npos * n.nword
			alphabetWords += n.nsym * n.nword
			exceptions += n.excCount
		}
	}
	t.Logf("patterns=%d frontendSyntaxParses=%d tryCompileNFA=%d forwardPositions=%d transitionWords=%d alphabetWords=%d exceptionRows=%d", m.n, parses, nfaAttempts, positions, transitionWords, alphabetWords, exceptions)
}

func TestRuntimePrefilterWork(t *testing.T) {
	requireDiag(t)
	db, _ := compileImageTest(t, getRequestImagePatterns(t), false)
	m := db.(*database).imageMVS()
	sc := &scratch{}
	data := bytes.Repeat([]byte("HTTP/1.1 200 OK\r\nServer: nginx\r\n\r\n"), 512)
	for i := 0; i < 2; i++ {
		hits := m.pf.scanHits(data, sc)
		t.Logf("scan=%d bytes=%d literals=%d literalHits=%d pairBufferLen=%d pairBufferCap=%d", i, len(data), len(m.literals), len(hits), len(sc.cpairs), cap(sc.cpairs))
	}
}
