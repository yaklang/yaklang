package tests

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// Regression tests for lazy-builder panics observed when compiling the
// nodejs/node repository (compile-log analysis 2026-09-08):
//
//  1. interface conversion panics: object-literal member names that are NOT
//     plain identifiers (string / numeric / computed keys on methods,
//     getters and setters) hit the unchecked ast.Node.AsIdentifier
//     assertion in VisitObjectLiteralExpression, and its fallback
//     Node.Text() panicked on ComputedPropertyName.
//  2. nil pointer panics: statements visited after the current basic block
//     was already finished (e.g. unreachable code after `return`) made
//     EmitCall/EmitIf return nil, which callers dereferenced.
//
// These panics are recovered by ssa.LazyBuilder.Build and only surface as
// "lazy builder panic" log lines (in production they silently drop the rest
// of the batch's function bodies), so the tests capture the log output and
// assert no such line was produced.

func parseJSNoPanic(t *testing.T, code string) {
	t.Helper()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage("js"))
	require.NoError(t, err)
	require.NotNil(t, prog)
	// force lazy builders (function bodies) to run: the original panics
	// surfaced inside ssa.LazyBuilder.Build during deferred compilation
	for _, f := range prog.Program.Funcs.Values() {
		f.Build()
	}

	out := buf.String()
	require.False(t, strings.Contains(out, "lazy builder panic"),
		"compile produced recovered panics:\n%s", out)
}

func TestObjectLiteralNonIdentifierMemberNames(t *testing.T) {
	parseJSNoPanic(t, `
const computed = "c";
const obj = {
  "str-method"() { return 1; },
  42() { return 2; },
  [computed]() { return 3; },
  get "str-get"() { return 4; },
  set "str-set"(v) { },
  get 43() { return 5; },
  set [computed](v) { },
};
`)
}

func TestObjectLiteralNonIdentifierKeysInValues(t *testing.T) {
	// minimatch/undici style: string / numeric keys inside object literals
	// passed as call arguments
	parseJSNoPanic(t, `
const opts = { "encoding": "utf8", 404: "not found", [k]: v };
configure({ "str": 1, 42: 2, [computed]: 3 });
`)
}

func TestUnreachableCallAfterReturn(t *testing.T) {
	parseJSNoPanic(t, `
function f(a) {
  return 1;
  a();
}
function g(a) {
  return a.then(x => x).catch(e => e);
  Promise.resolve(1).finally(() => {});
}
`)
}

func TestUnreachableIfAfterReturn(t *testing.T) {
	parseJSNoPanic(t, `
function f(a, b) {
  return 1;
  if (a) {
    b();
  } else if (b) {
    a();
  }
}
`)
}
