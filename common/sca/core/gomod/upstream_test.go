// Test materials derived from aquasecurity/go-dep-parser fb7eb3159bd5b83c24e5dbf29e15b4b7ca4618e9.
// Copyright (c) 2019 Teppei Fukuda. MIT license: testdata/LICENSE.
// Interface adapted to declarations: local replacements and Go 1.16 indirect
// declarations are retained (they are not installed components).
package gomod

import (
	"context"
	"os"
	"reflect"
	"testing"
)

func TestUpstreamGoModFixtures(t *testing.T) {
	const dep = "github.com/aquasecurity/go-dep-parser"
	const old = "v0.0.0-20211224170007-df43bca6b6ff"
	const replaced = "v0.0.0-20220406074731-71021a481237"
	const x = "golang.org/x/xerrors"
	const xv = "v0.0.0-20200804184101-5ec99f83aff1"
	const y = "gopkg.in/yaml.v3"
	const yv = "v3.0.0-20210107192922-496545a6307b"
	type expected struct {
		name, version, source string
		indirect              bool
	}
	full := []expected{{dep, old, "", false}, {x, xv, "", true}, {y, yv, "", true}}
	local := []expected{{dep, old, "", false}, {x, "", "./xerrors", true}, {y, yv, "", true}}
	for _, tc := range []struct {
		name string
		want []expected
	}{
		{"normal", full}, {"no-go-version", []expected{{dep, old, "", false}}},
		{"replaced", []expected{{dep, replaced, dep, false}, {x, xv, "", true}}},
		{"replaced-with-version", []expected{{dep, replaced, dep, false}, {x, xv, "", true}}},
		{"replaced-with-version-mismatch", full},
		{"replaced-with-local-path", local}, {"replaced-with-local-path-and-version", local}, {"replaced-with-local-path-and-version-mismatch", full},
		{"go116", []expected{{dep, old, "", false}, {y, yv, "", true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, e := os.ReadFile("testdata/" + tc.name + ".mod")
			if e != nil {
				t.Fatal(e)
			}
			f, e := Parse(context.Background(), data, Limits{})
			if e != nil {
				t.Fatal(e)
			}
			var got []expected
			for _, d := range f.Declarations() {
				s := ""
				if d.Replacement != nil {
					s = d.Replacement.New.Path
				}
				got = append(got, expected{d.Effective.Path, d.Effective.Version, s, d.Requirement.Indirect})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("declarations\ngot %#v\nwant %#v", got, tc.want)
			}
		})
	}
}

// Punctuation cases ported from x/mod modfile/read_test.go TestParsePunctuation;
// this tests the extracted lexer, not acceptance of these as require directives.
func TestUpstreamLexPunctuation(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{"require ()", []string{"require", "(", ")"}},
		{"require []{},", []string{"require", "[", "]", "{", "}", ","}},
		{"require a[b]c{d}e,", []string{"require", "a", "[", "b", "]", "c", "{", "d", "}", "e", ","}},
		{"require (\n\ta[b]\n)", []string{"require", "(", "a", "[", "b", "]", ")"}},
		{"require [v1.0.0, v1.1.0)", []string{"require", "[", "v1.0.0", ",", "v1.1.0", ")"}},
	} {
		in := &input{complete: []byte(tc.src), remaining: []byte(tc.src), ctx: context.Background(), limits: Limits{MaxTokenBytes: 65536}, pos: Position{Line: 1, LineRune: 1}}
		var got []string
		for {
			in.readToken()
			if in.token.kind == _EOF {
				break
			}
			if in.token.kind != '\n' {
				got = append(got, in.token.text)
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: %#v", tc.src, got)
		}
	}
}
func TestCanonicalModuleVersions(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"v1", "v1.0.0"}, {"v1.2", "v1.2.0"}, {"v1.2.3+build.9", "v1.2.3"}, {"v2.0.0+incompatible", "v2.0.0+incompatible"}} {
		f, e := Parse(context.Background(), []byte("require example.org/x "+tc.input), Limits{})
		if e != nil {
			t.Fatal(e)
		}
		if f.Require[0].Version != tc.want {
			t.Fatalf("%s: %s", tc.input, f.Require[0].Version)
		}
	}
	for _, s := range []string{"require example.org/x v2.0.0", "require example.org/x/v2 v1.0.0", "require gopkg.in/yaml.v3 v2.0.0", "require example.org/x v1.2.3-01", "require example.org/x v1.2.3+a..b"} {
		if _, e := Parse(context.Background(), []byte(s), Limits{}); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
