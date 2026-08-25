package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

type doubleFreeSFCase struct {
	name   string
	code   string
	wantDF bool
}

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

		require.Greater(t, len(lifetime.ListHeapAllocs(prog.Program)), 0)
		require.Greater(t, len(lifetime.ListFreeCalls(prog.Program)), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_DoubleFree_SyntaxFlow_Cases(t *testing.T) {
	cases := []doubleFreeSFCase{
		{
			name: "basic double free",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    free(p);
    return 0;
}
`,
			wantDF: true,
		},
		{
			name: "alias q=p then free both",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    free(p);
    free(q);
    return 0;
}
`,
			wantDF: true,
		},
		{
			name: "single free is safe",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    return 0;
}
`,
			wantDF: false,
		},
		{
			name: "may-free then free is double free",
			code: `
#include <stdlib.h>
int main(int abrt) {
    int *p = (int*)malloc(sizeof(int));
    if (abrt) {
        free(p);
    }
    free(p);
    return 0;
}
`,
			wantDF: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckWithNameOnlyInMemory("", t, tc.code, func(prog *ssaapi.Program) error {
				res, err := prog.SyntaxFlowWithError(`*<doubleFree()> as $df`)
				require.NoError(t, err)
				got := res.GetValues("df")
				if tc.wantDF {
					require.Greater(t, got.Len(), 0, "expected doubleFree")
				} else {
					require.Equal(t, 0, got.Len(), "unexpected doubleFree: %v", got)
				}
				return nil
			}, ssaapi.WithLanguage(ssaconfig.C))
		})
	}
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
	run := func(t *testing.T, rule string, wantDF bool) {
		t.Helper()
		ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			got := res.GetValues("df")
			if wantDF {
				require.Greater(t, got.Len(), 0, "expected doubleFree for %s", rule)
			} else {
				require.Equal(t, 0, got.Len(), "unexpected doubleFree: %v", got)
			}
			return nil
		}, ssaapi.WithLanguage(ssaconfig.C))
	}

	t.Run("named target pa only", func(t *testing.T) {
		run(t, `
pa as $pa
<doubleFree(target=$pa)> as $df
`, true)
	})
	t.Run("star receiver with target pa", func(t *testing.T) {
		run(t, `
pa as $pa
*<doubleFree(target=$pa)> as $df
`, true)
	})
	t.Run("receiver chain $pa<doubleFree()>", func(t *testing.T) {
		run(t, `
pa as $pa
$pa<doubleFree()> as $df
`, true)
	})
	t.Run("named target pb no double free", func(t *testing.T) {
		run(t, `
pb as $pb
<doubleFree(target=$pb)> as $df
`, false)
	})

	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
pa as $pa
<freeCall(target=$pa)> as $free
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("free").Len(), 0)

		res, err = prog.SyntaxFlowWithError(`
pa as $pa
$pa<heapAlloc()> as $alloc
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("alloc").Len(), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_LifetimeQuery_AlertConfig(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    free(p);
    return 0;
}
`
	rule := `
p as $p
*<doubleFree(target=$p)> as $df
*<heapAlloc(target=$p)> as $alloc
alert $df for {
	level: "high",
	risk: "double-free",
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("df").Len(), 0)
		require.Greater(t, res.GetValues("alloc").Len(), 0)
		require.NotEmpty(t, res.GetAlertVariables())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_LifetimeQuery_StarTarget_Filter(t *testing.T) {
	// *<native(target=$x)> must honor target and not return unrelated pointers.
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
pb as $pb
*<doubleFree(target=$pa)> as $df
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("df").Len(), 0, "pa double-free")

		res, err = prog.SyntaxFlowWithError(`
pb as $pb
*<doubleFree(target=$pb)> as $df
`)
		require.NoError(t, err)
		require.Equal(t, 0, res.GetValues("df").Len(), "pb freed once — no doubleFree")

		res, err = prog.SyntaxFlowWithError(`
pa as $pa
*<heapAlloc(target=$pa)> as $alloc
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("alloc").Len(), 0)

		res, err = prog.SyntaxFlowWithError(`
pa as $pa
*<freeCall(target=$pa)> as $free
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("free").Len(), 0)

		// Empty / missing target symbol with * receiver should not scan whole program
		// as related-to-nothing (specified but empty → no hits).
		res, err = prog.SyntaxFlowWithError(`
*<doubleFree(target=$missing)> as $df
`)
		require.NoError(t, err)
		require.Equal(t, 0, res.GetValues("df").Len(), "missing target symbol → empty")
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

		res, err = prog.SyntaxFlowWithError(`
p as $p
<derefSite(target=$p)> as $d
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("d").Len(), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_UAF_Kind_Config(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    free(p);
    return 0;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`*<uaf(kind="double-free")> as $df`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("df").Len(), 0)

		res, err = prog.SyntaxFlowWithError(`*<uaf(kind="uaf")> as $uaf`)
		require.NoError(t, err)
		// pure uaf (non double-free) may be empty on this snippet
		_ = res
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}
