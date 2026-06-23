package tests

import (
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/javasyntax"
)

// TestProbeTernaryDecompile decompiles a compiled probe class (set TERN_CLASS to its path) and
// prints the output, so we can see which ternary patterns become stubs ("multiple next").
func TestProbeTernaryDecompile(t *testing.T) {
	path := os.Getenv("TERN_CLASS")
	if path == "" {
		t.Skip("set TERN_CLASS")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	javaclassparser.EnableDecompileSyntaxValidation = false
	defer func() { javaclassparser.EnableDecompileSyntaxValidation = true }()
	out, derr := javaclassparser.Decompile(raw)
	if derr != nil {
		t.Fatalf("decompile error: %v", derr)
	}
	t.Logf("\n%s", out)
}

// TestProbeGrammarConstructs isolates which decompiler-emitted constructs the Java grammar
// rejects, so we know whether to fix the rendering (e.g. inner-class '$') or degrade the member.
func TestProbeGrammarConstructs(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"dollar-in-type", "class MAP { MAP$MAPX foo() { return null; } }"},
		{"dollar-method-name", "class MAP { int $(Object a) { return 0; } }"},
		{"dollar-type-and-method", "class MAP { public static MAP$MAPX $(Object a, Object b) { return null; } }"},
		{"dotted-inner-type", "class MAP { MAP.MAPX foo() { return null; } }"},
		{"interface-static-init", "interface I { static { int x = 1; } }"},
		{"class-static-init", "class C { static { int x = 1; } }"},
		{"field-eq-localvar", "class C { public static final int rNums = var0; }"},
		{"field-empty-slot", "class C { Object x = empty slot value; }"},
	}
	for _, c := range cases {
		err := javasyntax.Validate(c.src)
		if err != nil {
			t.Logf("[REJECT] %-24s : %s", c.name, firstReason(err.Error()))
		} else {
			t.Logf("[ACCEPT] %-24s", c.name)
		}
	}
}

func firstReason(s string) string {
	const marker = "reason: "
	if i := indexOf(s, marker); i >= 0 {
		rest := s[i+len(marker):]
		if j := indexOf(rest, "\n"); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	if j := indexOf(s, "\n"); j >= 0 {
		return s[:j]
	}
	return s
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
