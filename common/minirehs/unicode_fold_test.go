package minirehs

import (
	"fmt"
	"os"
	"reflect"
	"regexp/syntax"
	"strings"
	"testing"
)

func TestUnicodeFoldLiteralFiltering(t *testing.T) {
	for _, expr := range []string{`(?i)server[\w]+(?=!)`, `(?i)key[\w]+(?=!)`, `(?i)(?:kkserver|erver)[\w]+(?=!)`, `(?i)héllo[\w]+(?=!)`, `(?i)kkserverks[0-9]{2}`, `(?i)(?:kkserverks|erver)[0-9]{2}`, `(?i)server[0-9]{2}`, `(?i)kkkkkkkkkkkkheader[0-9]{2}`, `(?i)^kkkkheader[0-9]{2}$`, `(?i)^sss$`, `(?i)^kkk$`} {
		for _, locate := range []bool{false, true} {
			for _, kind := range []BackendKind{BackendMVS, BackendEngine} {
				t.Run(fmt.Sprintf("%s/%v/%s", kind, locate, expr), func(t *testing.T) {
					db, e := Compile([]Pattern{{ID: 1, Expr: expr}}, WithBackend(kind), WithReportLocation(locate), WithLogger(silentLogger{}))
					if e != nil {
						t.Fatal(e)
					}
					defer db.Close()
					candidates := []Database{db}
					if kind == BackendMVS {
						b, e := db.MarshalImage()
						if e != nil {
							t.Fatal(e)
						}
						loaded, e := LoadImage(b)
						if e != nil {
							t.Fatal(e)
						}
						defer loaded.Close()
						candidates = append(candidates, loaded)
					}
					oracle, e := Compile([]Pattern{{ID: 1, Expr: expr}}, WithBackend(BackendStdlib), WithLogger(silentLogger{}))
					if e != nil {
						t.Fatal(e)
					}
					defer oracle.Close()
					for _, input := range []string{"serveré!", "ſerveré!", "Keyé!", "KKſerveré!", "KKſerverKſ42", "HÉLLOé!", "héllo42!", "ſerver42", strings.Repeat("K", 12) + "header42", "KKKKheader42", "ſſſ", "KKK", "unrelated"} {
						for _, padding := range []string{"", strings.Repeat(" ", 1024)} {
							data := []byte(padding + input)
							want, e := imageMatches(oracle, data)
							if e != nil {
								t.Fatal(e)
							}
							if !locate && kind == BackendMVS {
								for i := range want {
									want[i].From, want[i].To = -1, -1
								}
								if len(want) > 1 {
									want = want[:1]
								}
							}
							for _, candidate := range candidates {
								got, e := imageMatches(candidate, data)
								if e != nil || !reflect.DeepEqual(got, want) {
									t.Fatalf("input=%q got=%v want=%v err=%v", input, got, want, e)
								}
							}
						}
					}
				})
			}
		}
	}
}

func TestUnicodeFoldWidth(t *testing.T) {
	re, err := syntax.Parse(`(?i)kkss`, syntax.Perl)
	if err != nil {
		t.Fatal(err)
	}
	if w, ok := maxByteWidth(re); !ok || w != 10 {
		t.Fatalf("width=%d bounded=%v", w, ok)
	}
	expr := `(?i)kkserverks`
	window := computeLitWindow(expr, []string{"erver"})
	if window.head != 13 || window.tail != 5 {
		t.Fatalf("fragment window=%+v", window)
	}
	if head := computeLitHeads(expr, []string{"erver"})["erver"]; head != 13 {
		t.Fatalf("fragment head=%d", head)
	}
	if cover := computeLitBiCover(expr, []string{"erver"})["erver"]; !cover.ok || cover.headF != 13 || cover.tailR != 5 {
		t.Fatalf("fragment cover=%+v", cover)
	}
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string][2]int{}
	literalTriggerBounds(parsed, map[string]struct{}{"erver": {}, "er": {}}, func(s string, h, tail int) { seen[s] = [2]int{h, tail} })
	if seen["erver"] != [2]int{13, 5} || seen["er"] != [2]int{13, 8} {
		t.Fatalf("overlapping fragment bounds=%v", seen)
	}
}

func TestUnicodeFoldPureLiteralIndex(t *testing.T) {
	for _, suffix := range []string{"", "-", "_"} {
		db, _ := compileImageTest(t, []Pattern{{ID: 1, Expr: `(?i)SK` + suffix}}, false)
		if db.Info().NumAlwaysOn != 0 {
			t.Fatal("short folded literal lost its index")
		}
		for _, s := range []string{"SK", "ſK", "SK", "ſK"} {
			got, e := imageMatches(db, []byte(s+suffix))
			if e != nil || len(got) != 1 {
				t.Fatalf("%q got=%v err=%v", s+suffix, got, e)
			}
		}
	}
	for n := 6; n <= 7; n++ {
		re, e := syntax.Parse(`(?i)`+strings.Repeat("k", n), syntax.Perl)
		if e != nil {
			t.Fatal(e)
		}
		lits := requiredLiterals(re)
		if n == 6 && len(lits) != 64 || n == 7 && len(lits) != 0 {
			t.Fatalf("expansion n=%d forms=%d", n, len(lits))
		}
	}
}

func FuzzUnicodeFoldFiltering(f *testing.F) {
	for _, seed := range []string{"server", "key", "kkk", "sss", "prefixsuffix"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed string) {
		if len(seed) == 0 || len(seed) > 16 {
			return
		}
		var original, folded strings.Builder
		for _, b := range []byte(seed) {
			r := byte('a' + b%26)
			if b == 'k' || b == 's' {
				r = b
			}
			original.WriteByte(r)
			switch r {
			case 'k':
				folded.WriteString("K")
			case 's':
				folded.WriteString("ſ")
			default:
				folded.WriteByte(r)
			}
		}
		expr := `(?i)^` + original.String() + `[\w]+(?=!)`
		db, err := Compile([]Pattern{{ID: 1, Expr: expr}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		got, err := imageMatches(db, []byte(folded.String()+"é!"))
		if err != nil || len(got) != 1 {
			t.Fatalf("expr=%q folded=%q got=%v err=%v", expr, folded.String(), got, err)
		}
	})
}

func TestLegacyImageUnicodeFold(t *testing.T) {
	for _, name := range []string{"compile-image-v1.rehs", "compile-image-v2.rehs", "compile-image-v3.rehs"} {
		b, e := os.ReadFile("testdata/" + name)
		if e != nil {
			t.Fatal(e)
		}
		db, e := LoadImage(b)
		if e != nil {
			t.Fatal(e)
		}
		defer db.Close()
		got, e := imageMatches(db, []byte("ſerver: abc"))
		if e != nil || len(got) != 1 || got[0].ID != 2 || got[0].From != -1 {
			t.Fatalf("%s got=%v err=%v", name, got, e)
		}
	}
}
