package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

const uafSFRule = `*<uaf()> as $uaf`

type uafSFCase struct {
	name    string
	code    string
	wantUAF bool
	// optional contain substrings of $uaf value strings (only when wantUAF)
	contain []string
}

func TestC_UAF_SyntaxFlow(t *testing.T) {
	cases := []uafSFCase{
		{
			name: "basic free then deref write",
			code: `
#include <stdlib.h>
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    *ptr = 10;
    free(ptr);
    *ptr = 20;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"20"},
		},
		{
			name: "arrow member after free",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    free(p);
    p->x = 1;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"1"},
		},
		{
			name: "copy alias q=p",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    free(p);
    *q = 3;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"3"},
		},
		{
			name: "if may-free then use after join",
			code: `
#include <stdlib.h>
int main(int abrt) {
    int *ptr = (int*)malloc(sizeof(int));
    *ptr = 10;
    if (abrt) {
        free(ptr);
    }
    *ptr = 20;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"20"},
		},
		{
			name: "if-else free in then use after join",
			code: `
#include <stdlib.h>
int main(int cond) {
    int *p = (int*)malloc(sizeof(int));
    if (cond) {
        free(p);
    } else {
        *p = 1;
    }
    *p = 2;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"2"},
		},
		{
			name: "if free in both branches then use",
			code: `
#include <stdlib.h>
int main(int cond) {
    int *p = (int*)malloc(sizeof(int));
    if (cond) {
        free(p);
    } else {
        free(p);
    }
    *p = 9;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"9"},
		},
		{
			name: "if free in then use only in else is safe",
			code: `
#include <stdlib.h>
int main(int cond) {
    int *p = (int*)malloc(sizeof(int));
    if (cond) {
        free(p);
    } else {
        *p = 1;
        free(p);
    }
    return 0;
}
`,
			wantUAF: false,
		},
		{
			name: "for free then use after loop",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int i;
    for (i = 0; i < 1; i++) {
        free(p);
    }
    *p = 7;
    return 0;
}
`,
			wantUAF: true,
			// loop latch may surface free/double-free; either counts as lifetime violation
		},
		{
			name: "for alloc use free inside body is safe",
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
			wantUAF: false,
		},
		{
			name: "while free then use after loop",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int i = 0;
    while (i < 1) {
        free(p);
        i++;
    }
    *p = 6;
    return 0;
}
`,
			wantUAF: true,
		},
		{
			name: "for free then use in later iteration",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int i;
    for (i = 0; i < 2; i++) {
        if (i == 0) {
            free(p);
        } else {
            *p = 5;
        }
    }
    return 0;
}
`,
			wantUAF: true,
		},
		{
			name: "if inside for free then use after for",
			code: `
#include <stdlib.h>
int main(int flag) {
    int *p = (int*)malloc(sizeof(int));
    int i;
    for (i = 0; i < 1; i++) {
        if (flag) {
            free(p);
        }
    }
    *p = 3;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"3"},
		},
		{
			name: "cross-func freep then use in caller",
			code: `
#include <stdlib.h>
void freep(int *p) {
    free(p);
}
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    freep(ptr);
    *ptr = 20;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"20"},
		},
		{
			name: "cross-func free then pass to user",
			code: `
#include <stdlib.h>
void touch(int *p) {
    *p = 4;
}
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    free(ptr);
    touch(ptr);
    return 0;
}
`,
			wantUAF: true,
		},
		{
			name: "cross-func wrapper free with alias",
			code: `
#include <stdlib.h>
void freep(int *p) {
    free(p);
}
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    freep(p);
    *q = 8;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"8"},
		},
		{
			name: "cross-func freep under if then use",
			code: `
#include <stdlib.h>
void freep(int *p) {
    free(p);
}
int main(int cond) {
    int *p = (int*)malloc(sizeof(int));
    if (cond) {
        freep(p);
    }
    *p = 11;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"11"},
		},
		{
			name: "safe use before free",
			code: `
#include <stdlib.h>
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    *ptr = 10;
    free(ptr);
    return 0;
}
`,
			wantUAF: false,
		},
		{
			name: "safe null after free",
			code: `
#include <stdlib.h>
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    free(ptr);
    ptr = 0;
    return 0;
}
`,
			wantUAF: false,
		},
		{
			name: "unrelated pointer not uaf",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *r = (int*)malloc(sizeof(int));
    free(p);
    *r = 1;
    free(r);
    return 0;
}
`,
			wantUAF: false,
		},
		{
			name: "cross-func safe freep without later use",
			code: `
#include <stdlib.h>
void freep(int *p) {
    free(p);
}
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    *ptr = 1;
    freep(ptr);
    return 0;
}
`,
			wantUAF: false,
		},
		// Step1: formal-parameter abstract objects (no malloc in callee).
		// Note: bare `*p = …` on a formal int* is lowered via PointerSideEffect and
		// does not attach a member-use on the Parameter in SSA; use call/arrow uses.
		{
			name: "param free then call use",
			code: `
#include <stdlib.h>
void sink(int *q);
void f(int *p) {
    free(p);
    sink(p);
}
`,
			wantUAF: true,
		},
		{
			name: "param free then arrow member write",
			code: `
#include <stdlib.h>
struct Node { int x; };
void f(struct Node *p) {
    free(p);
    p->x = 1;
}
`,
			wantUAF: true,
			contain: []string{"1"},
		},
		{
			name: "param may-free then call use after join",
			code: `
#include <stdlib.h>
void sink(int *q);
void f(int *p, int c) {
    if (c) {
        free(p);
    }
    sink(p);
}
`,
			wantUAF: true,
		},
		{
			name: "param use then free is safe",
			code: `
#include <stdlib.h>
void sink(int *q);
void f(int *p) {
    sink(p);
    free(p);
}
`,
			wantUAF: false,
		},
		{
			name: "param free without later use is safe",
			code: `
#include <stdlib.h>
void f(int *p) {
    free(p);
}
`,
			wantUAF: false,
		},
		// Double-free is a UAF subtype (second free of a Freed object).
		{
			name: "double free via int-star-star wrapper",
			code: `
#include <stdlib.h>
void free2(int **a) {
    free(*a);
}
int main() {
    int *p = (int*)malloc(20);
    free2(&p);
    free2(&p);
    return 0;
}
`,
			wantUAF: true,
		},
		{
			name: "double free basic",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    free(p);
    return 0;
}
`,
			wantUAF: true,
		},
		{
			name: "double free via alias q=p",
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
			wantUAF: true,
		},
		{
			name: "double free freep then free",
			code: `
#include <stdlib.h>
void freep(int *p) {
    free(p);
}
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep(p);
    free(p);
    return 0;
}
`,
			wantUAF: true,
		},
		{
			name: "double free on formal parameter",
			code: `
#include <stdlib.h>
void f(int *p) {
    free(p);
    free(p);
}
`,
			wantUAF: true,
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
			wantUAF: true,
		},
		{
			name: "free once is safe not double free",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 1;
    free(p);
    return 0;
}
`,
			wantUAF: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Memory-only: UAF lifetime markers are not fully restored via DB lazy-load yet.
			ssatest.CheckWithNameOnlyInMemory("", t, tc.code, func(prog *ssaapi.Program) error {
				res, err := prog.SyntaxFlowWithError(uafSFRule)
				require.NoError(t, err)
				got := res.GetValues("uaf")
				if !tc.wantUAF {
					require.Equal(t, 0, got.Len(), "unexpected UAF: %v", got)
					return nil
				}
				require.Greater(t, got.Len(), 0, "expected UAF findings")
				if len(tc.contain) > 0 {
					ssatest.CompareResult(t, true, res, map[string][]string{"uaf": tc.contain})
				}
				return nil
			}, ssaapi.WithLanguage(ssaconfig.C))
		})
	}
}

func TestC_UAF_LifetimeAPI(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    free(ptr);
    *ptr = 20;
    return 0;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		findings := lifetime.FindUAFUses(prog.Program)
		require.Greater(t, len(findings), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_UAF_LifetimeAPI_DoubleFree(t *testing.T) {
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
		findings := lifetime.FindUAFUses(prog.Program)
		require.Greater(t, len(findings), 0)
		hasDF := false
		for _, f := range findings {
			if f.Kind == lifetime.KindDoubleFree {
				hasDF = true
				require.NotNil(t, f.Use)
				require.Greater(t, f.FreedObj, int64(0))
			}
		}
		require.True(t, hasDF, "expected KindDoubleFree finding, got %#v", findings)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_UAF_SyntaxFlow_AlertConfig(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    free(ptr);
    *ptr = 20;
    return 0;
}
`
	rule := `
*<uaf()> as $uaf
alert $uaf for {
	level: "high",
	risk: "Use After Free",
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		require.Greater(t, res.GetValues("uaf").Len(), 0)
		require.NotEmpty(t, res.GetAlertVariables())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestC_UAF_SyntaxFlow_Target(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *pa = (int*)malloc(sizeof(int));
    int *pb = (int*)malloc(sizeof(int));
    int *safe = (int*)malloc(sizeof(int));
    *safe = 0;
    free(pa);
    *pa = 11;
    free(pb);
    *pb = 22;
    return 0;
}
`
	run := func(t *testing.T, rule string, wantContain, wantAbsent []string) {
		t.Helper()
		ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			got := res.GetValues("uaf")
			if len(wantContain) == 0 {
				require.Equal(t, 0, got.Len(), "unexpected UAF: %v", got)
				return nil
			}
			require.Greater(t, got.Len(), 0, "expected UAF for rule %s", rule)
			ssatest.CompareResult(t, true, res, map[string][]string{"uaf": wantContain})
			if len(wantAbsent) > 0 {
				all := got.String()
				for _, s := range wantAbsent {
					require.NotContains(t, all, s, "UAF should not include pointer %s", s)
				}
			}
			return nil
		}, ssaapi.WithLanguage(ssaconfig.C))
	}

	t.Run("named target pa only", func(t *testing.T) {
		run(t, `
pa as $pa
<uaf(target=$pa)> as $uaf
`, []string{"11"}, []string{"22"})
	})
	t.Run("star receiver with target pa", func(t *testing.T) {
		run(t, `
pa as $pa
*<uaf(target=$pa)> as $uaf
`, []string{"11"}, []string{"22"})
	})
	t.Run("receiver chain $pa<uaf()>", func(t *testing.T) {
		run(t, `
pa as $pa
$pa<uaf()> as $uaf
`, []string{"11"}, []string{"22"})
	})
	t.Run("named target pb only", func(t *testing.T) {
		run(t, `
pb as $pb
<uaf(target=$pb)> as $uaf
`, []string{"22"}, []string{"11"})
	})
	t.Run("safe pointer has no uaf", func(t *testing.T) {
		run(t, `
safe as $safe
<uaf(target=$safe)> as $uaf
`, nil, nil)
	})
}

func TestC_UAF_SyntaxFlow_Target_Alias(t *testing.T) {
	code := `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    free(p);
    *q = 3;
    return 0;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
q as $q
<uaf(target=$q)> as $uaf
`)
		require.NoError(t, err)
		got := res.GetValues("uaf")
		require.Greater(t, got.Len(), 0)
		ssatest.CompareResult(t, true, res, map[string][]string{"uaf": {"3"}})
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

// TestC_UAF_SyntaxFlow_Ex extends coverage toward known gaps (summary fixpoint,
// globals, null-clear, nested wrappers, bare *param, field alias).
// These encode *desired* behavior after future improvements; failures are expected
// until the corresponding capability lands — do not gate CI on this test alone.
func TestC_UAF_SyntaxFlow_Ex(t *testing.T) {
	cases := []uafSFCase{
		{
			// gap: free-param summary fixpoint (freep2 → freep → free)
			name: "nested wrapper freep2 then use",
			code: `
#include <stdlib.h>
void freep(int *p) { free(p); }
void freep2(int *p) { freep(p); }
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep2(p);
    *p = 7;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"7"},
		},
		{
			// gap: nested wrapper double-free via freep2 then free
			name: "nested wrapper freep2 then free is double free",
			code: `
#include <stdlib.h>
void freep(int *p) { free(p); }
void freep2(int *p) { freep(p); }
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep2(p);
    free(p);
    return 0;
}
`,
			wantUAF: true,
		},
		{
			// gap: p=NULL after free should clear dangling use (UAF Step3)
			name: "null after free then deref is safe",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    p = 0;
    *p = 1;
    return 0;
}
`,
			wantUAF: false,
		},
		{
			// gap: same-TU global pointer lifetime
			name: "global free then use",
			code: `
#include <stdlib.h>
int *g;
int main() {
    g = (int*)malloc(sizeof(int));
    free(g);
    *g = 3;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"3"},
		},
		// gap: cross-func global typestate — skip until kill of g in helper
		// propagates to use in main.
		// {
		// 	name: "global free in helper then use in main",
		// 	code: `
		// #include <stdlib.h>
		// int *g;
		// void killg(void) { free(g); }
		// int main() {
		//     g = (int*)malloc(sizeof(int));
		//     killg();
		//     *g = 4;
		//     return 0;
		// }
		// `,
		// 	wantUAF: true,
		// 	contain: []string{"4"},
		// },
		{
			// gap: bare *p on formal after free (c2ssa SideEffect / member attach)
			name: "param free then bare star write",
			code: `
#include <stdlib.h>
void f(int *p) {
    free(p);
    *p = 9;
}
`,
			wantUAF: true,
			contain: []string{"9"},
		},
		{
			// gap: struct field holding heap pointer, free via field then use
			name: "struct field free then use",
			code: `
#include <stdlib.h>
struct Box { int *buf; };
int main() {
    struct Box b;
    b.buf = (int*)malloc(sizeof(int));
    free(b.buf);
    *b.buf = 5;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"5"},
		},
		{
			// gap: realloc as free of old pointer when size 0 / move then use old
			name: "use old pointer after realloc move",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(8);
    int *q = (int*)realloc(p, 16);
    *p = 6;
    free(q);
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"6"},
		},
		{
			// gap: store freed pointer, later load and use (memory alias / store)
			name: "store freed ptr then load and use",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int **slot = (int**)malloc(sizeof(int*));
    free(p);
    *slot = p;
    *(*slot) = 8;
    free(slot);
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"8"},
		},
		{
			// desired: callee that only stores should ideally not be UAF; today may FP.
			// Document target: after "reads/writes" summary, storing-only is safe.
			name: "free then pass to store-only sink is safe target",
			code: `
#include <stdlib.h>
static int *held;
void hold(int *p) { held = p; }
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    hold(p);
    return 0;
}
`,
			wantUAF: false,
		},
		{
			// gap: three-level wrapper
			name: "triple wrapper freep3 then use",
			code: `
#include <stdlib.h>
void freep(int *p) { free(p); }
void freep2(int *p) { freep(p); }
void freep3(int *p) { freep2(p); }
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep3(p);
    *p = 2;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"2"},
		},
		{
			// gap: free via alias through function that frees q=p copy inside
			name: "wrapper frees local alias of param",
			code: `
#include <stdlib.h>
void freep(int *p) {
    int *q = p;
    free(q);
}
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep(p);
    *p = 11;
    return 0;
}
`,
			wantUAF: true,
			contain: []string{"11"},
		},
		{
			name: "double free via int-star-star wrapper ex",
			code: `
#include <stdlib.h>
void free2(int **a) {
    free(*a);
}
int main() {
    int *p = (int*)malloc(20);
    free2(&p);
    free2(&p);
    return 0;
}
`,
			wantUAF: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckWithNameOnlyInMemory("", t, tc.code, func(prog *ssaapi.Program) error {
				res, err := prog.SyntaxFlowWithError(uafSFRule)
				require.NoError(t, err)
				got := res.GetValues("uaf")
				if !tc.wantUAF {
					require.Equal(t, 0, got.Len(), "unexpected UAF: %v", got)
					return nil
				}
				require.Greater(t, got.Len(), 0, "expected UAF findings")
				if len(tc.contain) > 0 {
					ssatest.CompareResult(t, true, res, map[string][]string{"uaf": tc.contain})
				}
				return nil
			}, ssaapi.WithLanguage(ssaconfig.C))
		})
	}
}
