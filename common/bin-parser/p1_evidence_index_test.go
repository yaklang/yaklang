package bin_parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// The independent oracle keeps the old AST predicates. It computes every
// requested rule during the same walk so the parity test does not reintroduce
// one full source parse per scorecard into every ordinary test run.
func p1LegacyEvidenceForRules(root string, ruleFiles []string) (map[string]int, map[string]bool) {
	counts := make(map[string]int, len(ruleFiles))
	needles := make(map[string][]string, len(ruleFiles))
	for _, ruleFile := range ruleFiles {
		key := strings.ToLower(ruleKey(ruleFile))
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(ruleFile), ".yaml"))
		needles[ruleFile] = []string{key, base}
		for _, n := range yamlRootNodes(ruleFile) {
			needles[ruleFile] = append(needles[ruleFile], strings.ToLower(n))
		}
		counts[ruleFile] = 0
	}
	out := map[string]bool{}
	matches, _ := filepath.Glob(filepath.Join(root, "*_test.go"))
	fset := token.NewFileSet()
	for _, path := range matches {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if !strings.Contains(strings.ToLower(filepath.Base(path)), "fail") {
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if ok && sel.Sel != nil && sel.Sel.Name == "Run" && len(call.Args) >= 1 {
					bl, ok := call.Args[0].(*ast.BasicLit)
					if ok && bl.Kind == token.STRING {
						s, err := strconv.Unquote(bl.Value)
						if err == nil {
							low := strings.ToLower(s)
							if strings.Contains(low, "/") {
								for ruleFile, ruleNeedles := range needles {
									for _, nd := range ruleNeedles {
										if nd != "" && strings.Contains(low, nd) {
											counts[ruleFile]++
											break
										}
									}
								}
							}
						}
					}
				}
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok || (id.Name != "mustChild" && id.Name != "parseEthernet") {
				return true
			}
			for _, a := range call.Args {
				bl, ok := a.(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				s, err := strconv.Unquote(bl.Value)
				if err == nil && s != "" {
					out[s] = true
					if s == "HTTP Request" || s == "HTTP Response" {
						out["HTTP"] = true
					}
					if s == "TLS Record" || s == "Handshake" || s == "ClientHello" {
						out["TLS"] = true
					}
				}
			}
			return true
		})
	}
	return counts, out
}

// Keep the original per-rule scanner as a small-fixture oracle as well. Its
// repeated parsing is intentional here, but never scales with the real suite.
func p1LegacySuccessBranchRows(root, ruleFile string) int {
	key := strings.ToLower(ruleKey(ruleFile))
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(ruleFile), ".yaml"))
	needles := []string{key, base}
	for _, n := range yamlRootNodes(ruleFile) {
		needles = append(needles, strings.ToLower(n))
	}
	matches, _ := filepath.Glob(filepath.Join(root, "*_test.go"))
	fset := token.NewFileSet()
	count := 0
	for _, path := range matches {
		if strings.Contains(strings.ToLower(filepath.Base(path)), "fail") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Run" {
				return true
			}
			if len(call.Args) < 1 {
				return true
			}
			bl, ok := call.Args[0].(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(bl.Value)
			if err != nil {
				return true
			}
			low := strings.ToLower(s)
			if !strings.Contains(low, "/") {
				return true
			}
			for _, nd := range needles {
				if nd != "" && strings.Contains(low, nd) {
					count++
					break
				}
			}
			return true
		})
	}
	return count
}

func TestP1SourceEvidenceMatchesEveryRoadmapAndCard(t *testing.T) {
	seen := map[string]bool{}
	for _, item := range ProtocolRoadmap {
		if sc, ok := ResolveP0Scorecard(item.Name); ok && sc.Rule != "" {
			seen[sc.Rule] = true
		}
		if sc, ok := ResolveP1Scorecard(item.Name); ok && sc.Rule != "" {
			seen[sc.Rule] = true
		}
	}
	for _, cards := range [][]ProtocolScorecard{P0Scorecards, P1Scorecards} {
		for _, sc := range cards {
			if sc.Rule != "" {
				seen[sc.Rule] = true
			}
		}
	}
	ruleFiles := make([]string, 0, len(seen))
	for ruleFile := range seen {
		ruleFiles = append(ruleFiles, ruleFile)
	}
	sort.Strings(ruleFiles)
	require.NotEmpty(t, ruleFiles)
	wantRows, wantNames := p1LegacyEvidenceForRules(p1SourceEvidenceRoot(), ruleFiles)
	for _, ruleFile := range ruleFiles {
		require.Equal(t, wantRows[ruleFile], successBranchRows(ruleFile), ruleFile)
	}
	require.Equal(t, wantNames, p1MustChildNames())
}

func TestP1SourceEvidenceFixturesPreserveLiteralSemantics(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"alpha_test.go": `package fixture
func f() {
 t.Run("Proto/first", nil)
 t.Run("proto/first", nil)
 other.Run("proto/other", nil)
 t.Run(` + "`PROTO/raw`" + `, nil)
 t.Run("proto\x2farm", nil)
 t.Run("oprotoco/substring", nil)
 t.Run("proto", nil)
 t.Run("PROTO/" + branch, nil)
 t.Run(name, nil)
 t.Run(1, nil)
 t.Run()
 Run("proto/direct", nil)
 obj.run("proto/lower", nil)
 mustChild(n, "HTTP Request", "TLS Record", "Custom", 99)
 parseEthernet("Handshake", "ClientHello", "", "HTTP Response")
 obj.mustChild("Selector ignored")
 MustChild("Cased ignored")
 mustChild(named)
}
`,
		"beta_FAIL_test.go": `package fixture
func f() { t.Run("proto/excluded", nil); mustChild(n, "From fail file") }
`,
		"failish_test.go": `package fixture
func f() { t.Run("proto/also-excluded", nil); parseEthernet("From failish") }
`,
		"beta_test.go": `package fixture
func f() { t.Run("another/path", nil); t.Run("proto/first", nil); mustChild(n, "Custom") }
`,
		"malformed_test.go": `package fixture
func f() { t.Run("proto/malformed", nil); mustChild(n, "Malformed")
`,
		"ignored.go": `package fixture
func f() { t.Run("proto/non-test", nil); mustChild(n, "Non-test") }
`,
	}
	for name, source := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(source), 0600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "hidden_test.go"), []byte(`package fixture; func f() { t.Run("proto/nested", nil); mustChild("Nested") }`), 0600))
	index := scanP1SourceEvidence(dir)
	require.Equal(t, []string{"proto/first", "proto/first", "proto/other", "proto/raw", "proto/arm", "oprotoco/substring", "another/path", "proto/first"}, index.runNames)
	require.Equal(t, map[string]bool{
		"HTTP Request": true, "HTTP Response": true, "HTTP": true,
		"TLS Record": true, "Handshake": true, "ClientHello": true, "TLS": true,
		"Custom": true, "From fail file": true, "From failish": true,
	}, index.childNames)
	ruleFiles := []string{"proto.yaml", "PROTO.yaml", "application-layer/proto.yaml", "another.yaml", "absent.yaml", ".yaml"}
	wantRows, wantNames := p1LegacyEvidenceForRules(dir, ruleFiles)
	require.Equal(t, wantNames, index.childNames)
	for _, ruleFile := range ruleFiles {
		want := p1LegacySuccessBranchRows(dir, ruleFile)
		require.Equal(t, want, wantRows[ruleFile], ruleFile)
		require.Equal(t, want, index.successBranchRows(ruleFile), ruleFile)
	}
	require.Equal(t, 7, wantRows["proto.yaml"])
	require.Zero(t, wantRows[".yaml"])
	missing := scanP1SourceEvidence(filepath.Join(dir, "missing"))
	require.Empty(t, missing.runNames)
	require.Empty(t, missing.childNames)
}

func TestP1SourceEvidenceConcurrentReadersCannotMutateSnapshot(t *testing.T) {
	const readers = 16
	var wg sync.WaitGroup
	results := make([]map[string]bool, readers)
	rows := make([]int, readers)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = p1MustChildNames()
			rows[i] = successBranchRows("application-layer/spnego.yaml")
		}(i)
	}
	wg.Wait()
	for i := range results {
		require.Equal(t, results[0], results[i])
		require.Equal(t, rows[0], rows[i])
	}
	untouched := p1MustChildNames()
	require.NotEmpty(t, untouched)
	for name := range results[0] {
		delete(results[0], name)
		break
	}
	results[0]["caller-only marker"] = true
	require.Equal(t, untouched, p1MustChildNames())
	for i := 1; i < len(results); i++ {
		require.Equal(t, untouched, results[i])
	}
}

// This is the original p1MustChildNames scan, retained only as a benchmark
// oracle. Ordinary parity tests use the one-pass all-rules oracle above.
func p1LegacyMustChildNames(root string) map[string]bool {
	out := map[string]bool{}
	matches, _ := filepath.Glob(filepath.Join(root, "*_test.go"))
	fset := token.NewFileSet()
	for _, path := range matches {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok || (id.Name != "mustChild" && id.Name != "parseEthernet") {
				return true
			}
			for _, a := range call.Args {
				bl, ok := a.(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				s, err := strconv.Unquote(bl.Value)
				if err == nil && s != "" {
					out[s] = true
					if s == "HTTP Request" || s == "HTTP Response" {
						out["HTTP"] = true
					}
					if s == "TLS Record" || s == "Handshake" || s == "ClientHello" {
						out["TLS"] = true
					}
				}
			}
			return true
		})
	}
	return out
}

var p1EvidenceBenchmarkSink int

func BenchmarkP1SourceEvidenceFullCards(b *testing.B) {
	root := p1SourceEvidenceRoot()
	var ruleFiles []string
	for _, item := range ProtocolRoadmap {
		if item.Priority == priP1 {
			sc, ok := ResolveP1Scorecard(item.Name)
			if !ok {
				b.Fatalf("missing scorecard for %s", item.Name)
			}
			ruleFiles = append(ruleFiles, sc.Rule)
		}
	}
	// Check each per-rule result and the complete name map before measurement.
	// Deliberately retain aliases/repeated rules in the measured 144-card loop.
	wantRows, wantNames := p1LegacyEvidenceForRules(root, ruleFiles)
	index := scanP1SourceEvidence(root)
	for _, ruleFile := range ruleFiles {
		require.Equal(b, wantRows[ruleFile], index.successBranchRows(ruleFile), ruleFile)
	}
	require.Equal(b, wantNames, index.mustChildNames())
	require.Equal(b, wantNames, p1LegacyMustChildNames(root))
	wantTotal := len(wantNames)
	for _, ruleFile := range ruleFiles {
		wantTotal += wantRows[ruleFile]
	}
	b.Run("LegacyRepeatedScan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			total := len(p1LegacyMustChildNames(root))
			for _, ruleFile := range ruleFiles {
				total += p1LegacySuccessBranchRows(root, ruleFile)
			}
			if total != wantTotal {
				b.Fatalf("legacy result changed: %d != %d", total, wantTotal)
			}
			p1EvidenceBenchmarkSink = total
		}
	})
	b.Run("IndexedColdBuildAndFullCards", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Cold construction is intentionally inside the timer/allocation
			// accounting; this does not benchmark only a warmed sync.Once.
			index := scanP1SourceEvidence(root)
			total := len(index.mustChildNames())
			for _, ruleFile := range ruleFiles {
				total += index.successBranchRows(ruleFile)
			}
			if total != wantTotal {
				b.Fatalf("indexed result changed: %d != %d", total, wantTotal)
			}
			p1EvidenceBenchmarkSink = total
		}
	})
}
