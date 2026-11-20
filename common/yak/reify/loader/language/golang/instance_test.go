package golang

import (
	"strings"
	"testing"
)

func TestParseInstanceSplitsGenericNames(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pkg      string
		receiver string
		function string
		closure  string
		family   string
		args     []string
		generic  bool
	}{
		{
			name: "main.main", pkg: "main", function: "main", family: "main.main",
		},
		{
			name: "runtime.(*mspan).init", pkg: "runtime", receiver: "*mspan", function: "init",
			family: "runtime.(*mspan).init",
		},
		{
			// A value receiver is "pkg.Type.Method", as in archive/tar.
			name: "archive/tar.Header.allowedFormats", pkg: "archive/tar", receiver: "Header",
			function: "allowedFormats", family: "archive/tar.Header.allowedFormats",
		},
		{
			name:     "crypto/ecdsa.generateFIPS[go.shape.*crypto/internal/fips140/nistec.P224Point]",
			pkg:      "crypto/ecdsa",
			function: "generateFIPS",
			family:   "crypto/ecdsa.generateFIPS[T]",
			args:     []string{"go.shape.*crypto/internal/fips140/nistec.P224Point"},
			generic:  true,
		},
		{
			name:     "pkg.(*OrderedMap[go.shape.string,go.shape.int64]).Add",
			pkg:      "pkg",
			receiver: "*OrderedMap[T]",
			function: "Add",
			family:   "pkg.(*OrderedMap[T]).Add",
			args:     []string{"go.shape.string", "go.shape.int64"},
			generic:  true,
		},
		{
			// A slice argument keeps its own bracket pair inside the group.
			name:     "pkg.F[go.shape.string,go.shape.[]pkg.Value]",
			pkg:      "pkg",
			function: "F",
			family:   "pkg.F[T]",
			args:     []string{"go.shape.string", "go.shape.[]pkg.Value"},
			generic:  true,
		},
		{
			// An array argument nests a bracket pair too.
			name:     "pkg.F[go.shape.[4]int]",
			pkg:      "pkg",
			function: "F",
			family:   "pkg.F[T]",
			args:     []string{"go.shape.[4]int"},
			generic:  true,
		},
		{
			// A struct argument holds braces, spaces and semicolons.
			name:     "pkg.F[go.shape.struct { pkg.a int; pkg.b string }]",
			pkg:      "pkg",
			function: "F",
			family:   "pkg.F[T]",
			args:     []string{"go.shape.struct { pkg.a int; pkg.b string }"},
			generic:  true,
		},
		{
			name:     "pkg.F[go.shape.int].func1",
			pkg:      "pkg",
			function: "F",
			closure:  ".func1",
			family:   "pkg.F[T].func1",
			args:     []string{"go.shape.int"},
			generic:  true,
		},
		{
			name:     "pkg.F[go.shape.int].func1.2",
			pkg:      "pkg",
			function: "F",
			closure:  ".func1.2",
			family:   "pkg.F[T].func1.2",
			args:     []string{"go.shape.int"},
			generic:  true,
		},
		{
			name:     "pkg.(*T[go.shape.int]).M.func1",
			pkg:      "pkg",
			receiver: "*T[T]",
			function: "M",
			closure:  ".func1",
			family:   "pkg.(*T[T]).M.func1",
			args:     []string{"go.shape.int"},
			generic:  true,
		},
		{
			// A value receiver on a generic type keeps its brackets.
			name:     "pkg.T[go.shape.int].M",
			pkg:      "pkg",
			receiver: "T[T]",
			function: "M",
			family:   "pkg.T[T].M",
			args:     []string{"go.shape.int"},
			generic:  true,
		},
		{
			// A module path may contain dots; gosym reads "v2" as a receiver.
			name: "gopkg.in/yaml.v2.Unmarshal", pkg: "gopkg.in/yaml", receiver: "v2",
			function: "Unmarshal", family: "gopkg.in/yaml.v2.Unmarshal",
		},
		{
			// Compiler-owned names keep their brackets: this one is an array type.
			name: "type:.eq.[9]interface {}", family: "type:.eq.[9]interface {}",
		},
		{
			name: "go:buildid", family: "go:buildid",
		},
		{
			name:   "go:(*struct { enc.TextMarshaler }).MarshalText",
			family: "go:(*struct { enc.TextMarshaler }).MarshalText",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseInstance(tc.name)
			if got.Package != tc.pkg || got.Receiver != tc.receiver || got.Function != tc.function ||
				got.Closure != tc.closure || got.Family != tc.family || got.Generic != tc.generic {
				t.Fatalf("ParseInstance(%q) = %+v", tc.name, got)
			}
			if strings.Join(got.TypeArgs, "|") != strings.Join(tc.args, "|") {
				t.Fatalf("type args = %q, want %q", got.TypeArgs, tc.args)
			}
		})
	}
}

func TestParseInstanceKeepsFamilyStable(t *testing.T) {
	for _, name := range []string{
		"pkg.F[go.shape.int]",
		"pkg.F[go.shape.string]",
		"pkg.F[go.shape.[]pkg.Value]",
		"pkg.(*T[go.shape.int]).M",
		"pkg.(*T[go.shape.string]).M",
		"pkg.F[go.shape.int].func1",
	} {
		first := ParseInstance(name).Family
		if second := FamilyOf(first); second != first {
			t.Fatalf("FamilyOf(%q) = %q, want %q", first, second, first)
		}
	}
	if FamilyOf("pkg.F[go.shape.int]") != FamilyOf("pkg.F[go.shape.string]") {
		t.Fatal("two instances of one generic function must share a family")
	}
	if FamilyOf("pkg.(*T[go.shape.int]).M") == FamilyOf("pkg.(*T[go.shape.int]).N") {
		t.Fatal("distinct methods must not share a family")
	}
	if got := PackageOf("pkg.(*T[go.shape.int]).M"); got != "pkg" {
		t.Fatalf("PackageOf generic method = %q", got)
	}
}

func TestParseInstanceRejectsImbalancedBrackets(t *testing.T) {
	for _, name := range []string{"pkg.F[go.shape.int", "pkg.F]x[", "pkg.F[[2]int"} {
		instance := ParseInstance(name)
		if instance.Generic || instance.Package != "" || instance.Family != name {
			t.Fatalf("imbalanced %q split as %+v", name, instance)
		}
	}
}

func TestShapeHelpers(t *testing.T) {
	if got := ShapeName("go.shape.*pkg.Value"); got != "*pkg.Value" {
		t.Fatalf("ShapeName = %q", got)
	}
	hash := strings.Repeat("5c1f7d72", 8)
	if !IsShapeHash("go.shape." + hash) {
		t.Fatal("hashed shape not recognized")
	}
	for _, text := range []string{"go.shape.int", "go.shape." + strings.Repeat("zz", 32), "go.shape." + hash[:63]} {
		if IsShapeHash(text) {
			t.Fatalf("%q accepted as a hashed shape", text)
		}
	}
}

func FuzzParseInstance(f *testing.F) {
	f.Add("pkg.F[go.shape.int]")
	f.Add("pkg.(*T[go.shape.string,go.shape.int64]).M.func1")
	f.Add("pkg.F[[2]int]")
	f.Add("pkg.F[struct { x int }].func1")
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 4096 {
			return
		}
		instance := ParseInstance(name)
		// Re-parsing a family key must not invent a different family.
		if again := ParseInstance(instance.Family).Family; again != instance.Family {
			t.Fatalf("family %q is not stable, becomes %q", instance.Family, again)
		}
	})
}
