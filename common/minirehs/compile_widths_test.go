package minirehs

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"regexp/syntax"
	"strings"
	"testing"
)

func TestConcatWidthFacts(t *testing.T) {
	for _, expr := range []string{``, strings.Repeat(`[ab]token`, 32), `(?:[ab]token){32}`, `a*b[0-9]{3}中(?i)k(?:x?y?){3}end`, `(?:a?b?){3}x`, strings.Repeat(`(?i:k)[ab]token`, 8)} {
		raw, err := parseRegexSyntax(expr, syntax.Perl)
		if err != nil {
			t.Fatal(err)
		}
		for _, root := range []*syntax.Regexp{raw, raw.Simplify()} {
			var visit func(*syntax.Regexp)
			visit = func(re *syntax.Regexp) {
				if re.Op == syntax.OpConcat {
					before := analysisTreeShape(re)
					var storage [34]int
					prefix, suffix, ok := concatWidthSides(re.Sub, storage[:])
					if !ok {
						t.Fatal("valid width declined", expr)
					}
					for i := 0; i <= len(re.Sub); i++ {
						w, b := sumWidthRange(re.Sub, 0, i)
						x, y := widthSide(prefix[i])
						if w != x || b != y {
							t.Fatalf("prefix i=%d got=%d/%v want=%d/%v", i, x, y, w, b)
						}
						w, b = sumWidthRange(re.Sub, i, len(re.Sub))
						x, y = widthSide(suffix[i])
						if w != x || b != y {
							t.Fatalf("suffix i=%d got=%d/%v want=%d/%v", i, x, y, w, b)
						}
					}
					if len(prefix) != cap(prefix) || len(suffix) != cap(suffix) {
						t.Fatal("facts can append into adjacent side")
					}
					old := append([]int(nil), suffix...)
					_ = append(prefix, 123)
					if !reflect.DeepEqual(old, suffix) {
						t.Fatal("appended prefix corrupted suffix")
					}
					if before != analysisTreeShape(re) {
						t.Fatal("width facts edited AST")
					}
				}
				for _, sub := range re.Sub {
					visit(sub)
				}
			}
			visit(root)
		}
	}
	maxInt := int(^uint(0) >> 1)
	for _, subs := range [][]*syntax.Regexp{
		{{Op: syntax.OpRepeat, Min: 1, Max: maxInt, Sub: []*syntax.Regexp{{Op: syntax.OpLiteral, Rune: []rune{'a'}}}}, {Op: syntax.OpLiteral, Rune: []rune{'b'}}},
		{{Op: syntax.OpRepeat, Min: 1, Max: maxInt, Sub: []*syntax.Regexp{{Op: syntax.OpLiteral, Rune: []rune{'😀'}}}}},
	} {
		if _, _, ok := concatWidthSides(subs, nil); ok {
			t.Fatal("overflow must retain original summation path")
		}
	}
	p, s, ok := concatWidthSides(nil, nil)
	if !ok || len(p) != 1 || len(s) != 1 || p[0] != 0 || s[0] != 0 {
		t.Fatal("empty concat facts")
	}
}

func TestCompileWidthFactsEquivalent(t *testing.T) {
	old := compileWidthFactsEnabled
	defer func() { compileWidthFactsEnabled = old }()
	for _, expr := range []string{strings.Repeat(`[ab]token`, 8), strings.Repeat(`[ab]token`, 512), `(?:[ab]token){32}`, `(?:[ab]token){8}|.*suffix`, `(?:.*foo|bar.*)[ab]tail`, `(?i:k)[ab]token[ab]中[ab]word[ab]end`, `^` + strings.Repeat(`[ab]token`, 8) + `$`, `\b` + strings.Repeat(`[ab]token`, 8) + `\b`} {
		for _, locate := range []bool{false, true} {
			var dbs []Database
			var images [][]byte
			for _, enabled := range []bool{false, true} {
				compileWidthFactsEnabled = enabled
				db, err := Compile([]Pattern{{ID: 0, Expr: expr}}, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
				if err != nil {
					t.Fatal(err)
				}
				if db.(*database).imageMVS().nfas[0] == nil {
					db.Close()
					t.Fatal("width case must reach actual NFA")
				}
				image, err := db.MarshalImage()
				if err != nil {
					t.Fatal(err)
				}
				dbs = append(dbs, db)
				images = append(images, image)
			}
			if !bytes.Equal(images[0], images[1]) {
				t.Fatalf("width facts changed persisted plan: %q", expr)
			}
			loaded, err := LoadImage(images[1])
			if err != nil {
				t.Fatal(err)
			}
			dbs = append(dbs, loaded)
			for _, data := range [][]byte{nil, []byte(strings.Repeat("atoken", 512)), []byte("x foo atail suffix"), []byte("Katokena中bwordaend"), {0xff, 'a', 't', 'o', 'k', 'e', 'n', 0}} {
				oracle := programOracle([]Pattern{{ID: 0, Expr: expr}}, data, BackendMVS, locate, 0)
				if err := programCompare(programScan(t, dbs[1], data, 0), oracle, false); err != nil {
					t.Fatal(err)
				}
				for _, stop := range []int{0, 1, 2} {
					want := programScan(t, dbs[0], data, stop)
					for _, db := range dbs[1:] {
						if err := programCompare(programScan(t, db, data, stop), want, true); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			for _, db := range dbs {
				db.Close()
			}
		}
		// Direct readers cover raw counted-repeat, unknown sides, and Unicode
		// folds independently of whether a public route happens to choose heads.
		parsed, err := parseRegexSyntax(expr, syntax.Perl)
		if err != nil {
			t.Fatal(err)
		}
		tree := parsed.Simplify()
		lits := extractRequiredLiterals(tree, 2)
		compileWidthFactsEnabled = false
		w := computeLitWindowTree(tree, lits)
		h := computeLitHeads(expr, lits)
		bc := computeLitBiCover(expr, lits)
		compileWidthFactsEnabled = true
		if w != computeLitWindowTree(tree, lits) || !reflect.DeepEqual(h, computeLitHeads(expr, lits)) || !reflect.DeepEqual(bc, computeLitBiCover(expr, lits)) {
			t.Fatalf("analysis difference %q", expr)
		}
	}
}

func BenchmarkProgramWidthAnalysis(b *testing.B) {
	old := compileWidthFactsEnabled
	defer func() { compileWidthFactsEnabled = old }()
	for _, k := range []int{2, 8, 32, 128, 512} {
		b.Run(fmt.Sprint(k), func(b *testing.B) {
			tree, err := parseRegexSyntax(strings.Repeat(`[ab]token`, k), syntax.Perl)
			if err != nil {
				b.Fatal(err)
			}
			tree = tree.Simplify()
			for _, enabled := range []bool{false, true} {
				b.Run(map[bool]string{false: "Reference", true: "Facts"}[enabled], func(b *testing.B) {
					compileWidthFactsEnabled = enabled
					b.ReportAllocs()
					b.ResetTimer()
					var w litWindow
					for i := 0; i < b.N; i++ {
						w = computeLitWindowTree(tree, []string{"token"})
					}
					b.ReportMetric(float64(w.head), "head-bytes")
				})
			}
		})
	}
}

func TestProgramWidthEvidence(t *testing.T) {
	programEvidenceDir(t)
	if os.Getenv("MINIREHS_AST_MODE") != "reuse" {
		t.Fatal("width experiment requires prior exact AST reuse enabled")
	}
	old := compileWidthFactsEnabled
	compileWidthFactsEnabled = os.Getenv("MINIREHS_WIDTH_MODE") == "facts"
	defer func() { compileWidthFactsEnabled = old }()
	TestProgramCompileASTEvidence(t)
}

func BenchmarkProgramWidthCompile(b *testing.B) {
	old := compileWidthFactsEnabled
	defer func() { compileWidthFactsEnabled = old }()
	for _, set := range []string{"GetRequest", "MITM", "Wide128", "Wide512"} {
		b.Run(set, func(b *testing.B) {
			var pats []Pattern
			switch set {
			case "GetRequest":
				pats = getRequestImagePatterns(b)
			case "MITM":
				pats, _ = compilableMITMPatterns(b)
			case "Wide128":
				pats = []Pattern{{ID: 0, Expr: strings.Repeat(`[ab]token`, 128)}}
			case "Wide512":
				pats = []Pattern{{ID: 0, Expr: strings.Repeat(`[ab]token`, 512)}}
			}
			for _, enabled := range []bool{false, true} {
				b.Run(map[bool]string{false: "Reference", true: "Facts"}[enabled], func(b *testing.B) {
					compileWidthFactsEnabled = enabled
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
						if err != nil {
							b.Fatal(err)
						}
						db.Close()
					}
				})
			}
		})
	}
}
