package gomod

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestReadOnlyContract(t *testing.T) {
	input := `module example.org/app
 go 1.22.0
 require (
  "example.org/a" v1.2.3 // indirect
  example.org/b v2.0.0+incompatible
 )
 replace example.org/a v1.2.3 => ../a
 replace example.org/b => example.org/fork v2.1.0+incompatible
 exclude example.org/c v1.0.0
 retract [v1.0.0, v1.0.1]
 `
	f, err := Parse(context.Background(), []byte(input), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Module != "example.org/app" || f.Go != "1.22.0" {
		t.Fatalf("header: %#v", f)
	}
	want := []Requirement{{Path: "example.org/a", Version: "v1.2.3", Indirect: true, Line: 4}, {Path: "example.org/b", Version: "v2.0.0+incompatible", Line: 5}}
	if !reflect.DeepEqual(f.Require, want) {
		t.Fatalf("requirements: %#v", f.Require)
	}
	if len(f.Replace) != 2 || f.Replace[0].New.Path != "../a" || f.Replace[0].New.Version != "" || f.Replace[1].New.Version != "v2.1.0+incompatible" {
		t.Fatalf("replacements: %#v", f.Replace)
	}
	if len(f.Exclude) != 1 || len(f.Retract) != 1 {
		t.Fatalf("lost declarations: %#v", f)
	}
}

func TestMalformedAndBudgets(t *testing.T) {
	for _, s := range []string{"require (\na v1.0.0", "require x", "require x nope", "require x v1.0.0 extra", "require x v1.0.0\nrequire x v2.0.0", "replace x =>", "module a\nmodule b", "surprise attack", "require \"bad\\q\" v1.0.0", "require x v1.0.0 /* nope */", "require x\x00 v1.0.0", "require x\xff v1.0.0"} {
		t.Run(s, func(t *testing.T) {
			if _, e := Parse(context.Background(), []byte(s), Limits{}); e == nil {
				t.Fatalf("accepted %q", s)
			}
		})
	}
	for _, l := range []Limits{{MaxBytes: 1}, {MaxTokenBytes: 2}, {MaxStatements: 1}} {
		if _, e := Parse(context.Background(), []byte("module example.org/app\ngo 1.22"), l); !errors.Is(e, ErrLimit) {
			t.Fatalf("budget %#v: %v", l, e)
		}
	}
	if _, e := Parse(context.Background(), nil, Limits{MaxBytes: -1}); e == nil {
		t.Fatal("negative budget")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Parse(ctx, []byte("module m"), Limits{}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func TestQuotesCommentsAndNoFinalNewline(t *testing.T) {
	f, e := Parse(context.Background(), []byte("module `example.org/m`\r\nrequire (\r\n \"example.org/\\x61\" v1.2.3 // indirect; reason\r\n)"), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	if len(f.Require) != 1 || f.Require[0].Path != "example.org/a" || !f.Require[0].Indirect {
		t.Fatalf("%#v", f)
	}
}

// TestComments is the retained read-only remainder of x/mod TestComments:
// //indirect is kept as a declaration flag. Print/comment-block reconstruction
// is out of scope.
func TestComments(t *testing.T) {
	t.Run("quotes-indirect", TestQuotesCommentsAndNoFinalNewline)
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"", "module m\nrequire a v1.0.0", "require (\na v1.0.0\n)", "replace a => ../a", "retract [v1.0.0, v1.2.0]", "require \"\\q\" v1.0.0"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<18 {
			return
		}
		_, _ = Parse(context.Background(), []byte(s), Limits{MaxBytes: 1 << 18, MaxTokenBytes: 1 << 16, MaxStatements: 1000})
	})
}

func BenchmarkParse(b *testing.B) {
	for _, n := range []int{100, 1000, 10000, 100000} {
		var input strings.Builder
		input.WriteString("module m\nrequire (\n")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&input, "example.org/m%d v1.0.0\n", i)
		}
		input.WriteString(")\n")
		data := []byte(input.String())
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := Parse(context.Background(), data, Limits{MaxStatements: 200000}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestSumIdentityConflict(t *testing.T) {
	a := "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	b := "h1:AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	m, e := ParseSum(context.Background(), []byte("example.org/x v1.0.0 "+a+"\nexample.org/x v1.0.0/go.mod "+b+"\n"))
	if e != nil || len(m) != 2 {
		t.Fatal(m, e)
	}
	if _, e = ParseSum(context.Background(), []byte("example.org/x v1.0.0 "+a+"\nexample.org/x v1.0.0 "+b+"\n")); e == nil {
		t.Fatal("conflicting digest accepted")
	}
}

func TestParseSumEvidence(t *testing.T) {
	a := "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	src := "github.com/aquasecurity/go-dep-parser v0.0.0-20211224170007-df43bca6b6ff " + a + "\n" +
		"github.com/aquasecurity/go-dep-parser v0.0.0-20211224170007-df43bca6b6ff/go.mod " + a + "\n"
	m, e := ParseSum(context.Background(), []byte(src))
	if e != nil {
		t.Fatal(e)
	}
	mod := SumKey{Path: "github.com/aquasecurity/go-dep-parser", Version: "v0.0.0-20211224170007-df43bca6b6ff", GoMod: false}
	sum := SumKey{Path: "github.com/aquasecurity/go-dep-parser", Version: "v0.0.0-20211224170007-df43bca6b6ff", GoMod: true}
	if m[mod] != a || m[sum] != a || len(m) != 2 {
		t.Fatalf("sum evidence: %#v", m)
	}
}
