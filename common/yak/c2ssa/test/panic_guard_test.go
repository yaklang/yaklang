package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

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
