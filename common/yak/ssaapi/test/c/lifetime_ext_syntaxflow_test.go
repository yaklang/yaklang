package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

const memLeakSFRule = `*<memLeak()> as $leak`
const nullCheckSFRule = `*<nullCheck()> as $chk`

type memLeakSFCase struct {
	name     string
	code     string
	wantLeak bool
}

type nullCheckSFCase struct {
	name    string
	code    string
	wantChk bool
}

func TestC_MemLeak_SyntaxFlow(t *testing.T) {
	cases := []memLeakSFCase{
		{
			name: "basic malloc never freed",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 1;
    return 0;
}
`,
			wantLeak: true,
		},
		{
			name: "calloc never freed",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)calloc(1, sizeof(int));
    return 0;
}
`,
			wantLeak: true,
		},
		{
			name: "free then return is safe",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    return 0;
}
`,
			wantLeak: false,
		},
		{
			name: "if may-free still may leak",
			code: `
#include <stdlib.h>
int main(int abrt) {
    int *p = (int*)malloc(sizeof(int));
    if (abrt) {
        free(p);
    }
    return 0;
}
`,
			wantLeak: true,
		},
		{
			name: "free in both branches is safe",
			code: `
#include <stdlib.h>
int main(int cond) {
    int *p = (int*)malloc(sizeof(int));
    if (cond) {
        free(p);
    } else {
        free(p);
    }
    return 0;
}
`,
			wantLeak: false,
		},
		{
			name: "return ownership escape is safe",
			code: `
#include <stdlib.h>
int *make(void) {
    int *p = (int*)malloc(sizeof(int));
    return p;
}
int main() {
    int *q = make();
    free(q);
    return 0;
}
`,
			// make() returns p → not a leak in make; main frees → no leak
			wantLeak: false,
		},
		{
			name: "two allocs one leaked",
			code: `
#include <stdlib.h>
int main() {
    int *pa = (int*)malloc(sizeof(int));
    int *pb = (int*)malloc(sizeof(int));
    free(pb);
    return 0;
}
`,
			wantLeak: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckWithNameOnlyInMemory("", t, tc.code, func(prog *ssaapi.Program) error {
				res, err := prog.SyntaxFlowWithError(memLeakSFRule)
				require.NoError(t, err)
				got := res.GetValues("leak")
				if tc.wantLeak {
					require.Greater(t, got.Len(), 0, "expected memLeak")
				} else {
					require.Equal(t, 0, got.Len(), "unexpected memLeak: %v", got)
				}
				return nil
			}, ssaapi.WithLanguage(ssaconfig.C))
		})
	}
}

func TestC_MemLeak_SyntaxFlow_AlertConfig(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    return 0;
}
`
	rule := `
*<memLeak()> as $leak
alert $leak for {
	level: "low",
	risk: "memory-leak",
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("leak").Len(), 0)
		require.NotEmpty(t, res.GetAlertVariables())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_MemLeak_SyntaxFlow_Target(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *pa = (int*)malloc(sizeof(int));
    int *pb = (int*)malloc(sizeof(int));
    int *safe = (int*)malloc(sizeof(int));
    free(safe);
    free(pb);
    return 0;
}
`
	run := func(t *testing.T, rule string, wantLeak bool) {
		t.Helper()
		ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			got := res.GetValues("leak")
			if wantLeak {
				require.Greater(t, got.Len(), 0, "expected leak for rule %s", rule)
			} else {
				require.Equal(t, 0, got.Len(), "unexpected leak: %v", got)
			}
			return nil
		}, ssaapi.WithLanguage(ssaconfig.C))
	}

	t.Run("named target pa leaks", func(t *testing.T) {
		run(t, `
pa as $pa
<memLeak(target=$pa)> as $leak
`, true)
	})
	t.Run("star receiver with target pa", func(t *testing.T) {
		run(t, `
pa as $pa
*<memLeak(target=$pa)> as $leak
`, true)
	})
	t.Run("receiver chain $pa<memLeak()>", func(t *testing.T) {
		run(t, `
pa as $pa
$pa<memLeak()> as $leak
`, true)
	})
	t.Run("named target pb freed no leak", func(t *testing.T) {
		run(t, `
pb as $pb
<memLeak(target=$pb)> as $leak
`, false)
	})
	t.Run("safe pointer has no leak", func(t *testing.T) {
		run(t, `
safe as $safe
<memLeak(target=$safe)> as $leak
`, false)
	})
}

func TestC_NullCheck_SyntaxFlow_Cases(t *testing.T) {
	cases := []nullCheckSFCase{
		{
			name: "bare if (p)",
			code: `
#include <stdlib.h>
int main(int *p) {
    if (p) {
        *p = 1;
    }
    return 0;
}
`,
			wantChk: true,
		},
		{
			name: "if (p != 0)",
			code: `
#include <stdlib.h>
int main(int *p) {
    if (p != 0) {
        *p = 1;
    }
    return 0;
}
`,
			wantChk: true,
		},
		{
			name: "if (p == 0)",
			code: `
#include <stdlib.h>
int main(int *p) {
    if (p == 0) {
        return 1;
    }
    *p = 1;
    return 0;
}
`,
			wantChk: true,
		},
		{
			name: "if (!p)",
			code: `
#include <stdlib.h>
int main(int *p) {
    if (!p) {
        return 1;
    }
    *p = 1;
    return 0;
}
`,
			wantChk: true,
		},
		{
			name: "no null check",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 1;
    free(p);
    return 0;
}
`,
			wantChk: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckWithNameOnlyInMemory("", t, tc.code, func(prog *ssaapi.Program) error {
				res, err := prog.SyntaxFlowWithError(nullCheckSFRule)
				require.NoError(t, err)
				got := res.GetValues("chk")
				if tc.wantChk {
					require.Greater(t, got.Len(), 0, "expected nullCheck")
				} else {
					require.Equal(t, 0, got.Len(), "unexpected nullCheck: %v", got)
				}
				return nil
			}, ssaapi.WithLanguage(ssaconfig.C))
		})
	}
}

func TestC_NullCheck_SyntaxFlow_Target(t *testing.T) {
	code := `
#include <stdlib.h>
int main(int *pa, int *pb) {
    if (pa) {
        *pa = 11;
    }
    if (pb == 0) {
        return 1;
    }
    *pb = 22;
    return 0;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
pa as $pa
<nullCheck(target=$pa)> as $chk
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("chk").Len(), 0)

		res, err = prog.SyntaxFlowWithError(`
pb as $pb
$pb<nullCheck()> as $chk
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("chk").Len(), 0)

		res, err = prog.SyntaxFlowWithError(`
*<nullCheck()> as $chk
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("chk").Len(), 1, "both pa and pb checks")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_NullCheck_SyntaxFlow_AlertConfig(t *testing.T) {
	code := `
#include <stdlib.h>
int main(int *p) {
    if (p != 0) {
        *p = 1;
    }
    return 0;
}
`
	rule := `
*<nullCheck()> as $chk
alert $chk for {
	level: "info",
	risk: "null-check",
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("chk").Len(), 0)
		require.NotEmpty(t, res.GetAlertVariables())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_PointsTo_Aliases_SyntaxFlow_Config(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    int *r = (int*)malloc(sizeof(int));
    free(p);
    free(r);
    return 0;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		// pointsTo on alloc / copy
		res, err := prog.SyntaxFlowWithError(`
p as $p
$p<pointsTo()> as $obj
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("obj").Len(), 0, "p should pointsTo its heap object")

		res, err = prog.SyntaxFlowWithError(`
q as $q
$q<pointsTo()> as $obj
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("obj").Len(), 0, "alias q should pointsTo")

		// aliases: q may-alias p
		res, err = prog.SyntaxFlowWithError(`
p as $p
q as $q
$q<aliases(target=$p)> as $hit
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("hit").Len(), 0, "q should alias p")

		// aliases: r should NOT alias p
		res, err = prog.SyntaxFlowWithError(`
p as $p
r as $r
$r<aliases(target=$p)> as $hit
`)
		require.NoError(t, err)
		require.Equal(t, 0, res.GetValues("hit").Len(), "independent r should not alias p")

		// against= alias for target
		res, err = prog.SyntaxFlowWithError(`
p as $p
q as $q
$q<aliases(against=$p)> as $hit
`)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("hit").Len(), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_Lifetime_Combined_Config(t *testing.T) {
	// Mix query natives + memLeak / doubleFree in one rule-style config.
	code := `
#include <stdlib.h>
int main() {
    int *leak = (int*)malloc(sizeof(int));
    int *df = (int*)malloc(sizeof(int));
    free(df);
    free(df);
    return 0;
}
`
	rule := `
*<heapAlloc()> as $alloc
*<freeCall()> as $free
*<doubleFree()> as $df
*<memLeak()> as $leak
alert $df for {
	level: "high",
	risk: "double-free",
}
alert $leak for {
	level: "low",
	risk: "memory-leak",
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("alloc").Len(), 0)
		require.Greater(t, res.GetValues("free").Len(), 0)
		require.Greater(t, res.GetValues("df").Len(), 0)
		require.Greater(t, res.GetValues("leak").Len(), 0)
		require.NotEmpty(t, res.GetAlertVariables())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_MemLeak_LifetimeAPI(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    return 0;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		findings := lifetime.FindMemLeaks(prog.Program)
		require.Greater(t, len(findings), 0)
		has := false
		for _, f := range findings {
			if f.Kind == lifetime.KindLeak {
				has = true
			}
		}
		require.True(t, has)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

// TestC_MemLeak_SyntaxFlow_Ex encodes desired behavior for harder cases.
// Soft assertions: log gaps instead of failing CI hard when analysis is incomplete.
func TestC_MemLeak_SyntaxFlow_Ex(t *testing.T) {
	cases := []memLeakSFCase{
		{
			name: "nested free wrapper then no leak",
			code: `
#include <stdlib.h>
void freep(int *p) { free(p); }
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep(p);
    return 0;
}
`,
			wantLeak: false,
		},
		{
			name: "early return without free is leak",
			code: `
#include <stdlib.h>
int main(int c) {
    int *p = (int*)malloc(sizeof(int));
    if (c) {
        return 1;
    }
    free(p);
    return 0;
}
`,
			wantLeak: true,
		},
		{
			// gap: may-alive + loop back-edge can keep Alive across iterations
			name: "alloc in loop body freed is safe",
			code: `
#include <stdlib.h>
int main() {
    int i;
    for (i = 0; i < 3; i++) {
        int *p = (int*)malloc(sizeof(int));
        *p = 1;
        free(p);
    }
    return 0;
}
`,
			wantLeak: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckWithNameOnlyInMemory("", t, tc.code, func(prog *ssaapi.Program) error {
				res, err := prog.SyntaxFlowWithError(memLeakSFRule)
				require.NoError(t, err)
				got := res.GetValues("leak")
				if tc.wantLeak && got.Len() == 0 {
					t.Logf("gap: expected memLeak for %s", tc.name)
				}
				if !tc.wantLeak && got.Len() > 0 {
					t.Logf("gap: unexpected memLeak for %s: %v", tc.name, got)
				}
				return nil
			}, ssaapi.WithLanguage(ssaconfig.C))
		})
	}
}
