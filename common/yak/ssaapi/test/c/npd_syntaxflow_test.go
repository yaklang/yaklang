package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

const npdSFRule = `*<npd()> as $npd`

type npdSFCase struct {
	name    string
	code    string
	wantNPD bool
}

func TestC_NPD_SyntaxFlow(t *testing.T) {
	cases := []npdSFCase{
		{
			name: "null then arrow member write",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = 0;
    p->x = 1;
    return 0;
}
`,
			wantNPD: true,
		},
		{
			name: "null then arrow via nullptr",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = NULL;
    p->x = 2;
    return 0;
}
`,
			wantNPD: true,
		},
		{
			name: "may-null after if then arrow",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main(int c) {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    if (c) {
        p = 0;
    }
    p->x = 1;
    return 0;
}
`,
			wantNPD: true,
		},
		{
			name: "malloc use free is safe for npd",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    p->x = 1;
    free(p);
    return 0;
}
`,
			wantNPD: false,
		},
		{
			name: "null without deref is safe",
			code: `
#include <stdlib.h>
int main() {
    int *p = 0;
    return 0;
}
`,
			wantNPD: false,
		},
		{
			name: "null then call is not npd",
			code: `
#include <stdlib.h>
void sink(int *q);
int main() {
    int *p = 0;
    sink(p);
    return 0;
}
`,
			wantNPD: false,
		},
		{
			name: "free then deref is uaf not npd",
			code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    *p = 20;
    return 0;
}
`,
			wantNPD: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckWithNameOnlyInMemory("", t, tc.code, func(prog *ssaapi.Program) error {
				res, err := prog.SyntaxFlowWithError(npdSFRule)
				require.NoError(t, err)
				got := res.GetValues("npd")
				if !tc.wantNPD {
					require.Equal(t, 0, got.Len(), "unexpected NPD: %v", got)
					return nil
				}
				require.Greater(t, got.Len(), 0, "expected NPD findings")
				return nil
			}, ssaapi.WithLanguage(ssaconfig.C))
		})
	}
}

func TestC_NPD_LifetimeAPI(t *testing.T) {
	code := `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = 0;
    p->x = 1;
    return 0;
}
`
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		findings := lifetime.FindNPDUses(prog.Program)
		require.Greater(t, len(findings), 0)
		has := false
		for _, f := range findings {
			if f.Kind == lifetime.KindNPD {
				has = true
			}
		}
		require.True(t, has)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

// TestC_NPD_SyntaxFlow_Ex extends NPD coverage toward known gaps (bare *p,
// null guards, param null, alias, globals, field). Desired behavior after
// future work; failures are expected for now — exploratory only.
func TestC_NPD_SyntaxFlow_Ex(t *testing.T) {
	cases := []npdSFCase{
		{
			// gap: bare int* store after null (c2ssa often drops member-use)
			name: "null then bare star write",
			code: `
#include <stdlib.h>
int main() {
    int *p = 0;
    *p = 1;
    return 0;
}
`,
			wantNPD: true,
		},
		{
			// gap: load via bare *p after null
			name: "null then bare star read",
			code: `
#include <stdlib.h>
int main() {
    int *p = 0;
    int x = *p;
    return x;
}
`,
			wantNPD: true,
		},
		{
			// gap: if (p) guard should make deref safe
			name: "if nonnull guard then deref is safe",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main(int c) {
    struct Node *p = 0;
    if (c) {
        p = (struct Node*)malloc(sizeof(struct Node));
    }
    if (p) {
        p->x = 1;
    }
    return 0;
}
`,
			wantNPD: false,
		},
		{
			// gap: if (!p) then deref should be NPD
			name: "if null guard then deref is npd",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = 0;
    if (!p) {
        p->x = 1;
    }
    return 0;
}
`,
			wantNPD: true,
		},
		{
			// gap: formal param assigned null then arrow
			name: "param assign null then arrow",
			code: `
#include <stdlib.h>
struct Node { int x; };
void f(struct Node *p) {
    p = 0;
    p->x = 1;
}
`,
			wantNPD: true,
		},
		{
			// gap: copy alias q=p after null
			name: "null then alias q then arrow",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = 0;
    struct Node *q = p;
    q->x = 1;
    return 0;
}
`,
			wantNPD: true,
		},
		{
			// gap: global null then use
			name: "global null then arrow",
			code: `
#include <stdlib.h>
struct Node { int x; };
struct Node *g;
int main() {
    g = 0;
    g->x = 1;
    return 0;
}
`,
			wantNPD: true,
		},
		{
			// gap: field of struct is null pointer then deref
			name: "struct field null then star",
			code: `
#include <stdlib.h>
struct Box { int *buf; };
int main() {
    struct Box b;
    b.buf = 0;
    *b.buf = 1;
    return 0;
}
`,
			wantNPD: true,
		},
		{
			// gap: ternary may-null then arrow
			name: "ternary may-null then arrow",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main(int c) {
    struct Node *p = c ? 0 : (struct Node*)malloc(sizeof(struct Node));
    p->x = 1;
    return 0;
}
`,
			wantNPD: true,
		},
		{
			// gap: loop may leave p null then use after loop
			name: "for may set null then use after",
			code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    int i;
    for (i = 0; i < 1; i++) {
        if (i == 0) {
            p = 0;
        }
    }
    p->x = 1;
    return 0;
}
`,
			wantNPD: true,
		},
		{
			// desired: passing null to sink is not NPD (phase1 already); keep regression
			name: "null then sink still not npd",
			code: `
#include <stdlib.h>
void sink(int *q);
int main() {
    int *p = 0;
    sink(p);
    return 0;
}
`,
			wantNPD: false,
		},
		{
			// gap: return *p after null (load use)
			name: "null then return star",
			code: `
#include <stdlib.h>
int f(void) {
    int *p = 0;
    return *p;
}
`,
			wantNPD: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckWithNameOnlyInMemory("", t, tc.code, func(prog *ssaapi.Program) error {
				res, err := prog.SyntaxFlowWithError(npdSFRule)
				require.NoError(t, err)
				got := res.GetValues("npd")
				if !tc.wantNPD {
					require.Equal(t, 0, got.Len(), "unexpected NPD: %v", got)
					return nil
				}
				require.Greater(t, got.Len(), 0, "expected NPD findings")
				return nil
			}, ssaapi.WithLanguage(ssaconfig.C))
		})
	}
}
