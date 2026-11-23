package golang

import (
	"strings"
	"testing"
)

func TestParseInstanceSplitsGenericNames(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		display   string
		pkg       string
		receiver  string
		function  string
		closure   string
		family    string
		args      []string
		generic   bool
		synthetic bool
	}{
		{
			raw: "main.main", display: "main.main", pkg: "main", function: "main", family: "main.main",
		},
		{
			raw: "runtime.(*mspan).init", display: "runtime.(*mspan).init", pkg: "runtime", receiver: "*mspan", function: "init",
			family: "runtime.(*mspan).init",
		},
		{
			// A value receiver is "pkg.Type.Method", as in archive/tar.
			raw: "archive/tar.Header.allowedFormats", display: "archive/tar.Header.allowedFormats", pkg: "archive/tar", receiver: "Header",
			function: "allowedFormats", family: "archive/tar.Header.allowedFormats",
		},
		{
			raw:      "crypto/ecdsa.generateFIPS[go.shape.*crypto/internal/fips140/nistec.P224Point]",
			display:  "crypto/ecdsa.generateFIPS[*crypto/internal/fips140/nistec.P224Point]",
			pkg:      "crypto/ecdsa",
			function: "generateFIPS",
			family:   "crypto/ecdsa.generateFIPS[T]",
			args:     []string{"*crypto/internal/fips140/nistec.P224Point"},
			generic:  true,
		},
		{
			raw:      "pkg.(*OrderedMap[go.shape.string,go.shape.int64]).Add",
			display:  "pkg.(*OrderedMap[string,int64]).Add",
			pkg:      "pkg",
			receiver: "*OrderedMap[string,int64]",
			function: "Add",
			family:   "pkg.(*OrderedMap[T]).Add",
			args:     []string{"string", "int64"},
			generic:  true,
		},
		{
			// A slice argument keeps its own bracket pair inside the group.
			raw:      "pkg.F[go.shape.string,go.shape.[]pkg.Value]",
			display:  "pkg.F[string,[]pkg.Value]",
			pkg:      "pkg",
			function: "F",
			family:   "pkg.F[T]",
			args:     []string{"string", "[]pkg.Value"},
			generic:  true,
		},
		{
			// An array argument nests a bracket pair too.
			raw:      "pkg.F[go.shape.[4]int]",
			display:  "pkg.F[[4]int]",
			pkg:      "pkg",
			function: "F",
			family:   "pkg.F[T]",
			args:     []string{"[4]int"},
			generic:  true,
		},
		{
			// A struct argument holds braces, spaces and semicolons.
			raw:      "pkg.F[go.shape.struct { pkg.a int; pkg.b string }]",
			display:  "pkg.F[struct { pkg.a int; pkg.b string }]",
			pkg:      "pkg",
			function: "F",
			family:   "pkg.F[T]",
			args:     []string{"struct { pkg.a int; pkg.b string }"},
			generic:  true,
		},
		{
			raw:      "pkg.F[go.shape.int].func1",
			display:  "pkg.F[int].func1",
			pkg:      "pkg",
			function: "F",
			closure:  ".func1",
			family:   "pkg.F[T].func1",
			args:     []string{"int"},
			generic:  true,
		},
		{
			raw:      "pkg.F[go.shape.int].func1.2",
			display:  "pkg.F[int].func1.2",
			pkg:      "pkg",
			function: "F",
			closure:  ".func1.2",
			family:   "pkg.F[T].func1.2",
			args:     []string{"int"},
			generic:  true,
		},
		{
			raw:      "pkg.(*T[go.shape.int]).M.func1",
			display:  "pkg.(*T[int]).M.func1",
			pkg:      "pkg",
			receiver: "*T[int]",
			function: "M",
			closure:  ".func1",
			family:   "pkg.(*T[T]).M.func1",
			args:     []string{"int"},
			generic:  true,
		},
		{
			// A value receiver on a generic type keeps its brackets.
			raw:      "pkg.T[go.shape.int].M",
			display:  "pkg.T[int].M",
			pkg:      "pkg",
			receiver: "T[int]",
			function: "M",
			family:   "pkg.T[T].M",
			args:     []string{"int"},
			generic:  true,
		},
		{
			// Nested shapes lose the compiler prefix at every depth.
			raw:      "pkg.F[go.shape.[]go.shape.int]",
			display:  "pkg.F[[]int]",
			pkg:      "pkg",
			function: "F",
			family:   "pkg.F[T]",
			args:     []string{"[]int"},
			generic:  true,
		},
		{
			// A module path may contain dots; gosym reads "v2" as a receiver.
			raw: "gopkg.in/yaml.v2.Unmarshal", display: "gopkg.in/yaml.v2.Unmarshal", pkg: "gopkg.in/yaml", receiver: "v2",
			function: "Unmarshal", family: "gopkg.in/yaml.v2.Unmarshal",
		},
		{
			// Compiler-owned names keep their brackets: this one is an array type.
			raw: "type:.eq.[9]interface {}", display: "type:.eq.[9]interface {}", family: "type:.eq.[9]interface {}",
			synthetic: true,
		},
		{
			raw: "go:buildid", display: "go:buildid", family: "go:buildid", synthetic: true,
		},
		{
			raw: "go:(*struct { enc.TextMarshaler }).MarshalText", display: "go:(*struct { enc.TextMarshaler }).MarshalText",
			family: "go:(*struct { enc.TextMarshaler }).MarshalText", synthetic: true,
		},
		{
			// A method value wrapper is compiler-generated.
			raw: "pkg.(*T).M-fm", display: "pkg.(*T).M-fm", pkg: "pkg", receiver: "*T", function: "M-fm",
			family: "pkg.(*T).M-fm", synthetic: true,
		},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got := ParseInstance(tc.raw)
			if got.Raw != tc.raw || got.Name != tc.display || got.Package != tc.pkg || got.Receiver != tc.receiver ||
				got.Function != tc.function || got.Closure != tc.closure || got.Family != tc.family ||
				got.Generic != tc.generic || got.Synthetic != tc.synthetic {
				t.Fatalf("ParseInstance(%q) = %+v", tc.raw, got)
			}
			if strings.Join(got.TypeArgs, "|") != strings.Join(tc.args, "|") {
				t.Fatalf("type args = %q, want %q", got.TypeArgs, tc.args)
			}
		})
	}
}

func TestParseInstanceRendersHashedShape(t *testing.T) {
	hash := strings.Repeat("5c1f7d72", 8)
	name := "pkg.F[go.shape." + hash + "]"
	instance := ParseInstance(name)
	if instance.Name != "pkg.F[shape#5c1f7d72]" {
		t.Fatalf("display name = %q", instance.Name)
	}
	if len(instance.TypeArgs) != 1 || instance.TypeArgs[0] != "shape#5c1f7d72" {
		t.Fatalf("type args = %q", instance.TypeArgs)
	}
}

func TestInstanceKeepsRawTypeArgs(t *testing.T) {
	hash := strings.Repeat("5c1f7d72", 8)
	instance := ParseInstance("pkg.F[go.shape.int,go.shape." + hash + "]")
	if strings.Join(instance.RawTypeArgs, "|") != "go.shape.int|go.shape."+hash {
		t.Fatalf("raw type args = %q", instance.RawTypeArgs)
	}
	if strings.Join(instance.TypeArgs, "|") != "int|shape#5c1f7d72" {
		t.Fatalf("display type args = %q", instance.TypeArgs)
	}
	nested := ParseInstance("pkg.F[go.shape.[]go.shape.int]")
	if strings.Join(nested.RawTypeArgs, "|") != "go.shape.[]go.shape.int" {
		t.Fatalf("nested raw type args = %q", nested.RawTypeArgs)
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
	if got := DisplayOf("pkg.F[go.shape.int]"); got != "pkg.F[int]" {
		t.Fatalf("DisplayOf = %q", got)
	}
}

func TestParseInstanceRejectsImbalancedBrackets(t *testing.T) {
	for _, name := range []string{"pkg.F[go.shape.int", "pkg.F]x[", "pkg.F[[2]int"} {
		instance := ParseInstance(name)
		if instance.Generic || instance.Package != "" || instance.Family != name || instance.Name != name || instance.Raw != name {
			t.Fatalf("imbalanced %q split as %+v", name, instance)
		}
	}
}

func TestInstanceIdentity(t *testing.T) {
	instance := ParseInstance("pkg.(*T[go.shape.int]).M.func1")
	identity := instance.Identity()
	if identity == nil || identity.Package != "pkg" || identity.Receiver != "*T[int]" || identity.Function != "M" ||
		identity.Closure != ".func1" || identity.Family != "pkg.(*T[T]).M.func1" || !identity.Generic {
		t.Fatalf("identity = %+v", identity)
	}
	if len(identity.TypeArgs) != 1 || identity.TypeArgs[0] != "int" {
		t.Fatalf("identity type args = %q", identity.TypeArgs)
	}
	if identity.Synthetic {
		t.Fatal("a closure is not synthetic")
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
