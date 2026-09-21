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
		name, file string
		want       []expected
		replaced   bool
	}{
		{"normal", "normal.mod", full, false},
		{"no-replace", "normal.mod", full, false},
		{"no-go-version", "no-go-version.mod", []expected{{dep, old, "", false}}, false},
		{"replaced", "replaced.mod", []expected{{dep, replaced, dep, false}, {x, xv, "", true}}, true},
		{"replaced-with-version", "replaced-with-version.mod", []expected{{dep, replaced, dep, false}, {x, xv, "", true}}, true},
		{"replaced-with-version-mismatch", "replaced-with-version-mismatch.mod", full, false},
		{"replaced-with-local-path", "replaced-with-local-path.mod", local, true},
		{"replaced-with-local-path-and-version", "replaced-with-local-path-and-version.mod", local, true},
		{"replaced-with-local-path-and-version-mismatch", "replaced-with-local-path-and-version-mismatch.mod", full, false},
		{"go116", "go116.mod", []expected{{dep, old, "", false}, {y, yv, "", true}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, e := os.ReadFile("testdata/" + tc.file)
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
			hasReplace := false
			for _, d := range f.Declarations() {
				if d.Replacement != nil {
					hasReplace = true
					break
				}
			}
			if hasReplace != tc.replaced {
				t.Fatalf("replacement applied=%v want %v", hasReplace, tc.replaced)
			}
		})
	}
}

// TestModuleID keeps the go-dep-parser module-id cases as path@version
// declarations. Inventory versions are still stored without a leading v.
func TestModuleID(t *testing.T) {
	for _, tc := range []struct {
		name, path, version, want string
	}{
		{"normal", "github.com/aquasecurity/trivy", "v0.22.0", "github.com/aquasecurity/trivy@v0.22.0"},
		{"github.com/aquasecurity/trivy", "github.com/aquasecurity/trivy", "v0.22.0", "github.com/aquasecurity/trivy@v0.22.0"},
		{"pseudo-version", "github.com/aquasecurity/go-dep-parser", "v0.0.0-20211224170007-df43bca6b6ff", "github.com/aquasecurity/go-dep-parser@v0.0.0-20211224170007-df43bca6b6ff"},
		{"github.com/aquasecurity/go-dep-parser", "github.com/aquasecurity/go-dep-parser", "v0.0.0-20211224170007-df43bca6b6ff", "github.com/aquasecurity/go-dep-parser@v0.0.0-20211224170007-df43bca6b6ff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "module example.org/app\nrequire " + tc.path + " " + tc.version + "\n"
			f, e := Parse(context.Background(), []byte(src), Limits{})
			if e != nil {
				t.Fatal(e)
			}
			if Canonical(tc.version) == "" {
				t.Fatalf("canonical rejected %s", tc.version)
			}
			d := f.Declarations()
			if len(d) != 1 {
				t.Fatalf("declarations: %#v", d)
			}
			got := d[0].Effective.Path + "@" + d[0].Effective.Version
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
	t.Run("invalid-pseudo", func(t *testing.T) {
		if _, e := Parse(context.Background(), []byte("require github.com/aquasecurity/go-dep-parser v0.0.0-\n"), Limits{}); e == nil {
			t.Fatal("accepted malformed pseudo-version")
		}
	})
}

// Punctuation cases ported from x/mod modfile/read_test.go TestParsePunctuation;
// this tests the extracted lexer, not acceptance of these as require directives.
func TestUpstreamLexPunctuation(t *testing.T) {
	for _, tc := range []struct {
		desc, src string
		want      []string
	}{
		{"paren", "require ()", []string{"require", "(", ")"}},
		{"brackets", "require []{},", []string{"require", "[", "]", "{", "}", ","}},
		{"mix", "require a[b]c{d}e,", []string{"require", "a", "[", "b", "]", "c", "{", "d", "}", "e", ","}},
		{"block_mix", "require (\n\ta[b]\n)", []string{"require", "(", "a", "[", "b", "]", ")"}},
		{"interval", "require [v1.0.0, v1.1.0)", []string{"require", "[", "v1.0.0", ",", "v1.1.0", ")"}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
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
		})
	}
}

// TestModuleLine is the original x/mod ModulePath table applied to Parse.
// Original ModulePath is tolerant of junk after a module line; Parse is not.
// Cases with parseOK=false keep the original expected path as uncovered.
func TestModuleLine(t *testing.T) {
	for _, tc := range []struct {
		name, src, original string
		parseOK             bool
	}{
		{"quoted", `module "github.com/rsc/vgotest"`, "github.com/rsc/vgotest", true},
		{"plain", "module github.com/rsc/vgotest", "github.com/rsc/vgotest", true},
		{"quoted-spaces", `module  "github.com/rsc/vgotest"`, "github.com/rsc/vgotest", true},
		{"plain-spaces", "module  github.com/rsc/vgotest", "github.com/rsc/vgotest", true},
		{"raw-quotes", "module `github.com/rsc/vgotest`", "github.com/rsc/vgotest", true},
		{"quoted-v2", `module "github.com/rsc/vgotest/v2"`, "github.com/rsc/vgotest/v2", true},
		{"plain-v2", "module github.com/rsc/vgotest/v2", "github.com/rsc/vgotest/v2", true},
		{"quoted-yaml-v2", `module "gopkg.in/yaml.v2"`, "gopkg.in/yaml.v2", true},
		{"plain-yaml-v2", "module gopkg.in/yaml.v2", "gopkg.in/yaml.v2", true},
		{"quoted-nl", "module \"gopkg.in/check.v1\"\n", "gopkg.in/check.v1", true},
		{"quoted-broken-nl", "module \"gopkg.in/check.v1\n\"", "", false},
		{"plain-nl", "module gopkg.in/check.v1\n", "gopkg.in/check.v1", true},
		{"quoted-crlf", "module \"gopkg.in/check.v1\"\r\n", "gopkg.in/check.v1", true},
		{"plain-crlf", "module gopkg.in/check.v1\r\n", "gopkg.in/check.v1", true},
		{"quoted-blank", "module \"gopkg.in/check.v1\"\n\n", "gopkg.in/check.v1", true},
		{"plain-blank", "module gopkg.in/check.v1\n\n", "gopkg.in/check.v1", true},
		{"split-quoted", "module \n\"gopkg.in/check.v1\"\n\n", "", false},
		{"split-plain", "module \ngopkg.in/check.v1\n\n", "", false},
		{"quoted-junk", "module \"gopkg.in/check.v1\"asd", "", false},
		{"duplicate-module", "module  \nmodule a/b/c ", "a/b/c", false},
		{"quoted-only-spaces", `module "   "`, "   ", false},
		{"module-only", "module   ", "", false},
		{"quoted-padded-path", `module "  a/b/c  "`, "  a/b/c  ", false},
		{"comment", `module "github.com/rsc/vgotest1" // with a comment`, "github.com/rsc/vgotest1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(context.Background(), []byte(tc.src), Limits{})
			if tc.parseOK {
				if err != nil || f == nil || f.Module != tc.original {
					t.Fatalf("src %q: module=%v err=%v want %q", tc.src, f, err, tc.original)
				}
				return
			}
			if err == nil {
				t.Fatalf("src %q: Parse accepted; original ModulePath=%q", tc.src, tc.original)
			}
		})
	}
}

// TestModulePath is a local path-character check, not x/mod ModulePath extraction.
// Unicode and version-suffix path rules from x/mod are not claimed.
func TestModulePath(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"example.com/mod", true},
		{"github.com/a/b", true},
		{"", false},
		{"/abs", false},
		{"a b", false},
		{"a//b", false},
		{"a\\b", false},
		{"a\nb", false},
	} {
		name := tc.in
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			if modulePath(tc.in) != tc.want {
				t.Fatalf("modulePath(%q)=%v want %v", tc.in, modulePath(tc.in), tc.want)
			}
		})
	}
}

// TestParseVersions covers x/mod TestParseVersions Strict go/toolchain
// cases that apply to this grammar. ParseLax is out of scope.
func TestParseVersions(t *testing.T) {
	for _, tc := range []struct {
		desc, input string
		ok          bool
	}{
		{"empty", "module m\ngo \n", false},
		{"one", "module m\ngo 1\n", false},
		{"two", "module m\ngo 1.22\n", true},
		{"three", "module m\ngo 1.22.333\n", true},
		{"before", "module m\ngo v1.2\n", false},
		{"after", "module m\ngo 1.2rc1\n", true},
		{"space", "module m\ngo 1.2 3.4\n", false},
		{"alt1", "module m\ngo 1.2.3\n", true},
		{"alt2", "module m\ngo 1.2rc1\n", true},
		{"alt3", "module m\ngo 1.2beta1\n", true},
		{"alt4", "module m\ngo 1.2.beta1\n", false},
		{"tool", "module m\ntoolchain go1.2\n", true},
		{"tool1", "module m\ntoolchain go1.2.3\n", true},
		{"tool2", "module m\ntoolchain go1.2rc1\n", true},
		{"tool3", "module m\ntoolchain go1.2rc1-gccgo\n", true},
		{"tool4", "module m\ntoolchain default\n", true},
		{"tool5", "module m\ntoolchain inconceivable!\n", false},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := Parse(context.Background(), []byte(tc.input), Limits{})
			if (err == nil) != tc.ok {
				t.Fatalf("input %q err=%v want ok=%v", tc.input, err, tc.ok)
			}
		})
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
