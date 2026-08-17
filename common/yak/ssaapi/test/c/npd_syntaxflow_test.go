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
