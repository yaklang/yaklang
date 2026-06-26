package tests

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// TestGAPanicFreeBoundary locks in the panic-hardening of the GA push: each of these real-world
// classes previously aborted decompilation with a Go panic (nil-typed value dereference or stack
// underflow produced by incomplete stack simulation), which the recover net turned into a degraded
// method. They are pinned here from the industry .m2 corpus so the fixes never regress, and so the
// CI suite (which has no ~/.m2 access) still exercises these exact boundary shapes.
//
// Contract asserted for every case:
//   - Decompile must not panic and must not return an error.
//   - The output must parse as syntactically-valid Java (java2ssa frontend).
//   - Cases marked wantFull must additionally be fully reconstructed (no stub marker).
func TestGAPanicFreeBoundary(t *testing.T) {
	cases := []struct {
		file     string
		desc     string
		wantFull bool
	}{
		{
			file:     "panic_nil_argtype.class",
			desc:     "ant SelectorUtils.matchPath: an argument value with nil Type() flowed into FunctionCallExpression arg-cast logic (expression.go RawType() nil-deref)",
			wantFull: true,
		},
		{
			file:     "panic_nil_bintype.class",
			desc:     "ant CBZip2InputStream: a binary/unary expression built with a nil result type panicked at typ.Copy()",
			wantFull: true,
		},
		{
			file:     "panic_nil_mergetype.class",
			desc:     "bndlib HeaderReader: MergeTypes received a nil arm type and called String() on it",
			wantFull: true,
		},
		{
			file:     "panic_nil_arraytype.class",
			desc:     "ant CBZip2OutputStream: NewJavaArrayMember inspected a nil object Type() (IsArray/String nil-deref)",
			wantFull: true,
		},
		{
			file:     "panic_stack_underflow.class",
			desc:     "logback NestingType.$INIT: a DUP-family opcode peeked an empty operand stack (underflow); must degrade cleanly to a marked stub, never panic",
			wantFull: false,
		},
		{
			file:     "panic_nilref_floatingiowriter.class",
			desc:     "beetl FloatingIOWriter.<init>: a typed-nil *JavaRef reached varUserMap as a key (loadVarBySlot on an uninitialized slot) and the variable-fold walker dereferenced ref.VarUid; must degrade cleanly to a marked stub, never panic",
			wantFull: false,
		},
		{
			file:     "panic_nilref_typeutils.class",
			desc:     "fastjson2 TypeUtils.doubleValue: incomplete if-route metadata exposed nil TrueNode/FalseNode callbacks during merge inference and if rewriting; must degrade cleanly, never panic",
			wantFull: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := regressionFS.ReadFile("testdata/regression/" + tc.file)
			if err != nil {
				t.Fatalf("read embedded class %s failed: %v", tc.file, err)
			}
			// Decompile must not panic (a leaked panic would crash the test binary here).
			source, derr := javaclassparser.Decompile(raw)
			if derr != nil {
				t.Fatalf("decompile %s returned error (%s): %v", tc.file, tc.desc, derr)
			}
			if _, ferr := java2ssa.Frontend(source); ferr != nil {
				t.Fatalf("frontend parse failed for %s (%s): %v\n----- source -----\n%s", tc.file, tc.desc, ferr, source)
			}
			if tc.wantFull && strings.Contains(source, "yak-decompiler") {
				t.Fatalf("%s (%s): expected full decompilation, got a stub\n----- source -----\n%s", tc.file, tc.desc, source)
			}
		})
	}
}
