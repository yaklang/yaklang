package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func Test_UAF_Lifetime_RegisterAndDetect(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(4);
    free(p);
    *p = 1;
    return 0;
}
`
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.C))
	require.NoError(t, err)
	require.NotNil(t, prog)

	findings := lifetime.FindUAFUses(prog.Program)
	require.Greater(t, len(findings), 0, "expected at least one UAF finding")

	hasUAF := false
	for _, f := range findings {
		if f.Kind == "uaf" {
			hasUAF = true
			require.NotNil(t, f.Use)
			require.Greater(t, f.FreedObj, int64(0))
		}
	}
	require.True(t, hasUAF)
}

func Test_UAF_Lifetime_NoFalsePositiveSimple(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(4);
    *p = 1;
    free(p);
    return 0;
}
`
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.C))
	require.NoError(t, err)
	findings := lifetime.FindUAFUses(prog.Program)
	for _, f := range findings {
		require.NotEqual(t, "uaf", f.Kind, "unexpected UAF: %+v use=%v", f, f.Use)
	}
}

func Test_UAF_Lifetime_ParamFreeThenUse(t *testing.T) {
	code := `
#include <stdlib.h>
void sink(int *q);
void f(int *p) {
    free(p);
    sink(p);
}
`
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.C))
	require.NoError(t, err)
	findings := lifetime.FindUAFUses(prog.Program)
	require.Greater(t, len(findings), 0, "expected UAF on formal parameter")
	hasUAF := false
	for _, f := range findings {
		if f.Kind == "uaf" {
			hasUAF = true
		}
	}
	require.True(t, hasUAF)
}
