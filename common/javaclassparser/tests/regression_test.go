package tests

import (
	"embed"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// TestDecompileSwitchCaseOrder guards against the switch case-mapping corruption caused by the
// invalid sort.Slice comparators (always returning true) that scrambled the switch successor order
// at both the opcode level (CalcOpcodeStackInfo) and the rewriter level (SwitchRewriter). The bug
// produced syntactically-valid but semantically-wrong output (each case mapped to the wrong body),
// which the syntax safety net cannot catch, so we assert the case -> body mapping explicitly.
func TestDecompileSwitchCaseOrder(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/switch_case_order.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
	// pick(int): case 0 -> "zero", case 1 -> "one", case 2 -> "two", default -> "many".
	// The bug reversed this ordering, so verify the bodies appear in the correct relative order.
	order := []string{`"zero"`, `"one"`, `"two"`, `"many"`}
	prev := -1
	for _, lit := range order {
		idx := strings.Index(source, lit)
		if idx < 0 {
			t.Fatalf("expected output to contain %s\n----- source -----\n%s", lit, source)
		}
		if idx <= prev {
			t.Fatalf("switch case bodies out of order (%s appeared too early) - case mapping corrupted\n----- source -----\n%s", lit, source)
		}
		prev = idx
	}
}

//go:embed testdata/regression/*.class
var regressionFS embed.FS

// TestDecompileSyntaxRegression decompiles a set of real-world .class files that previously
// produced syntactically-invalid Java, and asserts the decompiled output now parses cleanly
// through the java2ssa frontend. Each entry guards a specific decompiler fix.
func TestDecompileSyntaxRegression(t *testing.T) {
	cases := []struct {
		file string
		desc string
		// substrings that MUST appear in the decompiled output (positive assertions)
		mustContain []string
		// substrings that MUST NOT appear (the previously-buggy rendering)
		mustNotContain []string
	}{
		{
			file: "lambda_methodref.class",
			desc: "invokedynamic lambda metafactory: method references and lambda expressions",
			// constructor reference must use ::new, static method reference must be Class::method
			mustContain: []string{"::new", "GeoTileGridAggregation::setupGeoTileGridAggregationDeserializer"},
			// the impl method must not be inlined as a full method declaration in argument position
			mustNotContain: []string{"::<init>", "ObjectBuilderDeserializer.lazy(GeoTileGridAggregation$Builder::new,protected"},
		},
		{
			file:           "annotation_classvalue.class",
			desc:           "annotation class-valued element renders as Type.class",
			mustContain:    []string{".class"},
			mustNotContain: []string{"LaQute/bnd/signing/JartoolSigner$Config;"},
		},
		{
			file:           "enum_subclass.class",
			desc:           "synthetic enum-constant subclass rendered as a normal class",
			mustNotContain: []string{"enum FileMagicNumber$"},
		},
		{
			file:           "string_escape_esc.class",
			desc:           "ESC control char escaped as \\u001b instead of \\x1b",
			mustNotContain: []string{`\x1b`},
		},
		{
			file:           "string_escape_cesu8.class",
			desc:           "CESU-8 / invalid bytes escaped as \\u00XX instead of \\xXX",
			mustNotContain: []string{`\xed`, `\xa1`},
		},
		{
			file:           "string_escape_vtab.class",
			desc:           "vertical tab escaped as \\u000b instead of \\v",
			mustNotContain: []string{`\v`},
		},
		{
			file:           "array_classliteral.class",
			desc:           "array class literal rendered as T[].class",
			mustContain:    []string{"[].class"},
		},
		{
			file:           "catch_primitive_type.class",
			desc:           "catch clause with degraded primitive type falls back to Throwable",
			mustNotContain: []string{"catch(boolean", "catch(int", "catch(long", "catch(double"},
		},
		{
			file: "decompile_stub_partial.class",
			desc: "post-increment side effect inside a ternary folds into `x++` and the method decompiles fully",
			// hasNext() and next() both decompile; next() is `(cond) ? (this.index++) : (... - this.index++)`
			mustContain:    []string{"boolean hasNext()", "int next()", "this.index++"},
			mustNotContain: []string{"yak-decompiler", "throw new RuntimeException"},
		},
		{
			file: "empty_slot_stub.class",
			desc: "incomplete stack simulation leaking 'empty slot value' degrades the method to a stub instead of emitting invalid code",
			// the offending method is stubbed; the internal placeholder never reaches the output
			mustContain:    []string{"yak-decompiler"},
			mustNotContain: []string{"empty slot value"},
		},
		{
			file:           "module_info.class",
			desc:           "module-info synthetic descriptor renders as a valid compilation unit, not `class module-info {}`",
			mustNotContain: []string{"class module-info"},
		},
		{
			file:           "float_double_consts.class",
			desc:           "float/double constant fields render as valid Java literals with F/D suffix",
			mustContain:    []string{"3.14F", "2.718281828D", "-1.5F"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := regressionFS.ReadFile("testdata/regression/" + tc.file)
			if err != nil {
				t.Fatalf("read embedded class %s failed: %v", tc.file, err)
			}
			source, err := javaclassparser.Decompile(raw)
			if err != nil {
				t.Fatalf("decompile %s failed (%s): %v", tc.file, tc.desc, err)
			}
			// the decompiled output must parse as syntactically-valid Java
			if _, ferr := java2ssa.Frontend(source); ferr != nil {
				t.Fatalf("frontend parse failed for %s (%s): %v\n----- decompiled source -----\n%s", tc.file, tc.desc, ferr, source)
			}
			for _, must := range tc.mustContain {
				if !strings.Contains(source, must) {
					t.Errorf("%s (%s): expected output to contain %q\n----- decompiled source -----\n%s", tc.file, tc.desc, must, source)
				}
			}
			for _, mustNot := range tc.mustNotContain {
				if strings.Contains(source, mustNot) {
					t.Errorf("%s (%s): expected output NOT to contain %q\n----- decompiled source -----\n%s", tc.file, tc.desc, mustNot, source)
				}
			}
		})
	}
}
