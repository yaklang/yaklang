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
