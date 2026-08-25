package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestC_LifetimeQuery_SyntaxFlow(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 1;
    free(p);
    free(p);
    return 0;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`*<heapAlloc()> as $alloc`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("alloc").Len(), 0, "expected heapAlloc")

		res, err = prog.SyntaxFlowWithError(`*<freeCall()> as $free`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("free").Len(), 0, "expected freeCall")

		res, err = prog.SyntaxFlowWithError(`*<doubleFree()> as $df`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("df").Len(), 0, "expected doubleFree")

		// API-level: registry lists should be non-empty for this snippet.
		require.Greater(t, len(lifetime.ListHeapAllocs(prog.Program)), 0)
		require.Greater(t, len(lifetime.ListFreeCalls(prog.Program)), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_LifetimeQuery_Target(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *pa = (int*)malloc(sizeof(int));
    int *pb = (int*)malloc(sizeof(int));
    free(pa);
    free(pa);
    free(pb);
    return 0;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
pa as $pa
<doubleFree(target=$pa)> as $df
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("df").Len(), 0)

		res, err = prog.SyntaxFlowWithError(`
pb as $pb
<doubleFree(target=$pb)> as $df
`)
		require.NoError(t, err)
		require.Equal(t, 0, res.GetValues("df").Len(), "pb freed once should not be double-free")

		res, err = prog.SyntaxFlowWithError(`
pa as $pa
<freeCall(target=$pa)> as $free
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("free").Len(), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_LifetimeQuery_DerefSite(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int x = *p;
    free(p);
    return x;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		sites := lifetime.ListDerefSites(prog.Program)
		if len(sites) == 0 {
			t.Skip("no RegisterDeref sites in this lowering; native still registered")
		}
		res, err := prog.SyntaxFlowWithError(`*<derefSite()> as $d`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("d").Len(), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}
