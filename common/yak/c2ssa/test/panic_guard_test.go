package test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func parseCRejectLazyPanic(t *testing.T, code string) *ssaapi.Program {
	t.Helper()
	var buf bytes.Buffer
	prevLevel := ssalog.Log.Level
	ssalog.Log.SetLevel("error")
	ssalog.Log.SetOutput(io.MultiWriter(os.Stderr, &buf))
	t.Cleanup(func() {
		ssalog.Log.Level = prevLevel
		ssalog.Log.SetOutput(os.Stderr)
	})

	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.C))
	require.NoError(t, err)
	require.NotNil(t, prog)
	out := buf.String()
	if strings.Contains(out, "lazy builder panic") {
		t.Fatalf("lazy builder panic while compiling C:\n%s", out)
	}
	return prog
}

func expectNoPanicPrint(t *testing.T, name, code string, expect []string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("unexpected panic: %v", r)
			}
		}()
		test.CheckPrintlnValue(code, expect, t)
	})
}

func TestCastedCallDoesNotPanic(t *testing.T) {
	t.Run("casted-function-call", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int add(int a, int b) {
    return a + b;
}

int main() {
    int value = (int)add(1, 2);
    println(value);
    return 0;
}
`, []string{"castType(number, Function-add(1,2))"}, t)
	})
}

func TestSideEffectGuards(t *testing.T) {
	expectNoPanicPrint(t, "missing-parameter-side-effect", `
#include <stdio.h>
#include <string.h>

void copy_to_null(char *src) {
	char *dest = 0;
	strcpy(dest, src);
	println("done");
}

int main() {
	copy_to_null("input");
	println("after");
	return 0;
}
`, []string{`"done"`, `"after"`})

	expectNoPanicPrint(t, "pointer-member-side-effect", `
#include <stdio.h>
#include <string.h>

struct Data {
	char name[32];
};

void append_extra(struct Data *d, const char *extra) {
	strcat(d->name, extra);
	println("extended");
}

int main() {
	struct Data *d = 0;
	append_extra(d, "x");
	println("after");
	return 0;
}
`, []string{`"extended"`, `"after"`})
}

func TestSideEffectReplaceValueMissingOperandDoesNotPanic(t *testing.T) {
	call := ssa.NewCall(ssa.NewUndefined(""), ssa.Values{}, nil, nil)
	orig := ssa.NewConst(1)
	se := ssa.NewSideEffect("se", call, orig)
	missing := ssa.NewConst(2)

	require.NotPanics(t, func() {
		se.ReplaceValue(missing, orig)
	})

	require.Equal(t, call.GetId(), se.CallSite)
	require.Equal(t, orig.GetId(), se.Value)
}

func TestPostfixLvalueAssignDoesNotPanic(t *testing.T) {
	// Nested `ptr->field.sub =` used to nil-deref in
	// buildPostfixSuffixLvalueFromPostfixSuffix when PeekValue of the
	// intermediate member returned nil (libevent-style code).
	t.Run("nested-member-assign", func(t *testing.T) {
		parseCRejectLazyPanic(t, `
struct timeval {
	long tv_sec;
	long tv_usec;
};

struct event_base {
	int n_common_timeouts;
	struct timeval max_dispatch_time;
};

struct event_base *event_base_new_with_config(void) {
	struct event_base *base;
	if ((base = (struct event_base *)0) == 0) {
		return 0;
	}
	base->max_dispatch_time.tv_sec = -1;
	base->n_common_timeouts = 0;
	return base;
}
`)
	})

	t.Run("incomplete-nested-member-assign", func(t *testing.T) {
		parseCRejectLazyPanic(t, `
struct Outer;
void evbuffer_add_cb(struct Outer *o) {
	o->inner.x = 99;
}
`)
	})

	t.Run("paren-arrow-assign", func(t *testing.T) {
		parseCRejectLazyPanic(t, `
struct Data { int value; };
void f(struct Data *d) {
	(d)->value = 42;
}
`)
	})

	t.Run("cast-arrow-nested-assign", func(t *testing.T) {
		parseCRejectLazyPanic(t, `
struct Inner { int x; };
struct Outer { struct Inner inner; };
void evhttp_connection_new_(void *p) {
	((struct Outer *)p)->inner.x = 7;
}
`)
	})

	t.Run("array-then-member-assign", func(t *testing.T) {
		parseCRejectLazyPanic(t, `
struct T { int n; };
void evbuffer_expand_fast_(struct T *arr) {
	arr[0].n = 1;
}
`)
	})

	expectNoPanicPrint(t, "nested-member-assign-value", `
#include <stdio.h>

struct Inner {
	int x;
};

struct Outer {
	struct Inner inner;
};

int main() {
	struct Outer outer = { .inner = { .x = 5 } };
	struct Outer *p = &outer;
	p->inner.x = 99;
	println(p->inner.x);
	return 0;
}
`, []string{"99"})
}

func TestBinOpReplaceValueMissingOperandDoesNotPanic(t *testing.T) {
	left := ssa.NewConst(10)
	right := ssa.NewConst(20)
	bin := ssa.NewBinOp(ssa.OpAnd, left, right)
	missing := ssa.NewConst(30)

	require.NotPanics(t, func() {
		bin.ReplaceValue(missing, left)
	})

	require.Equal(t, left.GetId(), bin.X)
	require.Equal(t, right.GetId(), bin.Y)

	replacement := ssa.NewConst(40)
	require.NotPanics(t, func() {
		bin.ReplaceValue(left, replacement)
	})
	require.Equal(t, replacement.GetId(), bin.X)
}
