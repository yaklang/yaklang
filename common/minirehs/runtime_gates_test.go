//go:build cgo

package minirehs

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	pcre2 "github.com/VillanCh/go-pcre2-lite/regexp2"
)

var runtimePCREClasses = []string{
	`[\w.-]`, `[^\w]`, `[\d]`, `[^\D]`, `[\s]`, `[^\S]`,
	`[\h]`, `[\H]`, `[\v]`, `[\V]`, `[\p{L}]`, `[^\p{N}]`, `[\P{N}]`,
	`[[:alpha:]]`, `[^[:digit:]]`, `[[:^alpha:]]`, `[][\w]`, `[\]\w]`,
	`[\Q]\E\w]`, `[^\Q]\E\w]`, `[\x{61}\w]`,
}

// The entire bracket atom must be widened, especially under negation. These
// patterns force PCRE2 (lookahead) so Go's intentional verifier priority cannot
// conceal a UCP/ASCII mismatch.
func TestPCRE2BracketSuperset(t *testing.T) {
	for _, class := range runtimePCREClasses {
		t.Run(class, func(t *testing.T) {
			expr := `prefix` + class + `+suffix(?=!)`
			native, err := pcre2.Compile(expr, pcre2.None)
			if err != nil {
				t.Fatal(err)
			}
			super, widened, ok := re2SupersetEx(expr)
			if !ok || !widened {
				t.Fatalf("no safe superset: %q", super)
			}
			re, err := regexp.Compile(super)
			if err != nil {
				t.Fatal(err)
			}
			positives := 0
			for _, middle := range []string{"a", "é", "٣", "中", "9", " ", "\t", "\n", "]", "[", "😀", "aé٣", "\x00"} {
				input := "prefix" + middle + "suffix!"
				hit, err := native.MatchString(input)
				if err != nil {
					t.Fatal(err)
				}
				if hit {
					positives++
					if !re.MatchString(input) {
						t.Fatalf("superset shrank: expr=%q super=%q input=%q", expr, super, input)
					}
				}
			}
			if positives == 0 {
				t.Fatal("test did not exercise a positive native match")
			}
		})
	}
	// Parentheses inside quoted/POSIX bracket atoms cannot end a lookaround.
	for _, expr := range []string{`prefix(?=[[:alpha:]()]x)[\w]+suffix`, `prefix(?=[\Q)\E\w]x)[\w]+suffix`} {
		if super, _, ok := re2SupersetEx(expr); !ok {
			t.Fatalf("group scanner lost bracket boundaries: %q => %q", expr, super)
		}
	}
}

func TestPCRE2LocalizedGates(t *testing.T) {
	pats := []Pattern{
		{ID: 1, Expr: `common-prefix[0-9]{2}(?=token)token`},
		{ID: 2, Expr: `^common-prefix[0-9]{2}(?=token)token`},
		{ID: 3, Expr: `\Acommon-prefix[0-9]{2}(?=token)token`},
		{ID: 4, Expr: `(?m)^common-prefix[0-9]{2}(?=token)token`},
		{ID: 5, Expr: `\bcommon-prefix[0-9]{2}(?=token)token`},
		{ID: 6, Expr: `(?<=left)common-prefix[0-9]{2}(?=token)token`},
		{ID: 7, Expr: `common-prefix([0-9]{2})token\1`},
		{ID: 8, Expr: `common-prefix[^\d]+token(?=!)`},
		{ID: 9, Expr: `common-prefix[\d]+token(?=!)`},
	}
	inputs := [][]byte{
		nil, []byte("unrelated"), []byte("common-prefix42token42"),
		[]byte("common-prefixXXtoken common-prefix42token42"),
		[]byte(" common-prefix42token42"), []byte("x\ncommon-prefix42token42"),
		[]byte("中common-prefix42token42"), []byte("😀common-prefix42token42"),
		[]byte("leftcommon-prefix42token42"), []byte("common-prefixé中token!"),
		[]byte("common-prefix٣٢token!"), []byte("common-prefix42token!"),
		append([]byte{0x80, ' '}, []byte("common-prefix42token42")...),
		append(bytes.Repeat([]byte("zqxv "), 4000), []byte(" common-prefixXXtoken common-prefix42token42")...),
	}
	for _, locate := range []bool{false, true} {
		t.Run(fmt.Sprint(locate), func(t *testing.T) {
			db, encoded := compileImageTest(t, pats, locate)
			loaded, err := LoadImage(encoded)
			if err != nil {
				t.Fatal(err)
			}
			defer loaded.Close()
			for _, candidate := range []Database{db, loaded} {
				m := candidate.(*database).imageMVS()
				if !m.gate[0] || m.gateHead[0] < 0 || m.gateHead[5] >= 0 {
					t.Fatal("expected localized gate and full-context lookbehind")
				}
				for _, input := range inputs {
					var want []Match
					for _, p := range pats {
						v := &pcre2Verifier{expr: p.Expr}
						if len(v.findAll(input)) > 0 {
							want = append(want, Match{ID: p.ID, From: -1, To: -1})
						}
					}
					got, err := imageMatches(candidate, input)
					if err != nil || !reflect.DeepEqual(got, want) {
						t.Fatalf("input=%q got=%v want=%v err=%v", input, got, want, err)
					}
				}
			}
		})
	}
}

func TestMVSMergedAnchoredStartMask(t *testing.T) {
	for _, width := range []int{6, 70} {
		for _, combined := range []bool{false, true} {
			pats := []Pattern{{ID: 1, Expr: fmt.Sprintf(`[0-9]{%d}`, width)}, {ID: 2, Expr: `^(a)\1$`}, {ID: 3, Expr: `^ab$`}}
			if combined {
				pats = append(pats, Pattern{ID: 4, Expr: `\b[0-9]{2}\b`})
			}
			db, _ := compileImageTest(t, pats, false)
			m := db.(*database).imageMVS()
			if m.kernel == nil || m.merged == nil {
				t.Fatal("C merged path unavailable")
			}
			for _, input := range []string{"aa", "ab", "0", "123456", "00", "xaa", strings.Repeat("x", 128), "中aa"} {
				sc := &scratch{}
				fast := append([]int(nil), m.kernel.mergedScan([]byte(input), sc)...)
				scalar := append([]int(nil), m.kernel.mergedScanScalar([]byte(input), sc)...)
				goHits := m.merged.scanExist([]byte(input), make([]bool, len(pats)), nil)
				sort.Ints(fast)
				sort.Ints(scalar)
				sort.Ints(goHits)
				if !reflect.DeepEqual(fast, scalar) || !reflect.DeepEqual(fast, goHits) {
					t.Fatalf("merged fast=%v scalar=%v Go=%v input=%q", fast, scalar, goHits, input)
				}
				var want []Match
				for _, p := range pats {
					v := &pcre2Verifier{expr: p.Expr}
					if len(v.findAll([]byte(input))) > 0 {
						want = append(want, Match{ID: p.ID, From: -1, To: -1})
					}
				}
				got, err := imageMatches(db, []byte(input))
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("combined=%v input=%q got=%v want=%v err=%v", combined, input, got, want, err)
				}
			}
			batch := m.kernel.mergedScanBatch([]byte("aaabxaa"), []int32{0, 2, 4, 7}, &scratch{})
			// Native merged output contains candidates: the widened (a)\1
			// gate also accepts "ab" until PCRE2 rejects it. Literal "ab"
			// itself uses the separate literal-triggered path.
			wantBatch := [][2]int32{{0, 1}, {1, 1}}
			if !reflect.DeepEqual(batch, wantBatch) {
				t.Fatalf("batch got=%v want=%v", batch, wantBatch)
			}
		}
	}
}

func FuzzPCRE2BracketSuperset(f *testing.F) {
	type check struct {
		native   *pcre2.Regexp
		superset *regexp.Regexp
	}
	checks := make([]check, len(runtimePCREClasses))
	for i, class := range runtimePCREClasses {
		expr := `prefix` + class + `+suffix(?=!)`
		native, err := pcre2.Compile(expr, pcre2.None)
		if err != nil {
			f.Fatal(err)
		}
		super, ok := re2Superset(expr)
		if !ok {
			f.Fatal(expr)
		}
		re, err := regexp.Compile(super)
		if err != nil {
			f.Fatal(err)
		}
		checks[i] = check{native, re}
		f.Add(byte(i), "aé٣中 \n]😀")
		f.Add(byte(i), "é")
	}
	f.Fuzz(func(t *testing.T, class byte, middle string) {
		if len(middle) > 512 {
			return
		}
		c := checks[int(class)%len(checks)]
		subject := "prefix" + middle + "suffix!"
		hit, err := c.native.MatchString(subject)
		if err != nil {
			t.Fatal(err)
		}
		if hit && !c.superset.MatchString(subject) {
			t.Fatalf("class=%s super=%q subject=%q", runtimePCREClasses[int(class)%len(checks)], c.superset, subject)
		}
	})
}
