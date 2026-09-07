package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// NPD native category.
const npdNativeRule = `
$focus<npd()> as $npd
alert $npd for { level: "high", risk: "null-pointer-deref" }
`

const npdScanRule = `*<npd()> as $npd`

func TestC_NPD_Config(t *testing.T) {
	runPtrNativeConfigCases(t, []ptrNativeConfigCase{
		{
			Name: "null then arrow member write",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = 0;
    p->x = 1;
    return 0;
}
`,
			Rule: `p as $focus` + npdNativeRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
			WantAlerts: []string{"npd"},
		},
		{
			Name: "null then arrow via nullptr",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = NULL;
    p->x = 2;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
		},
		{
			Name: "may-null after if then arrow",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main(int c) {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    if (c) { p = 0; }
    p->x = 1;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
		},
		{
			Name: "malloc use free is safe for npd",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    p->x = 1;
    free(p);
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 0, Max: 0}},
		},
		{
			Name: "null without deref is safe",
			Code: `
#include <stdlib.h>
int main() {
    int *p = 0;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 0, Max: 0}},
		},
		{
			Name: "null then call is not npd",
			Code: `
#include <stdlib.h>
void sink(int *q);
int main() {
    int *p = 0;
    sink(p);
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 0, Max: 0}},
		},
		{
			Name: "free then deref is uaf not npd",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    *p = 20;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 0, Max: 0}},
		},
		{
			Name: "multilevel int** NPD on null",
			Code: `
#include <stdlib.h>
int main() {
    int **pp = 0;
    **pp = 1;
    return 0;
}
`,
			Rule: `pp as $focus` + npdNativeRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
		},
		{
			Name: "struct field null then star",
			Code: `
#include <stdlib.h>
struct Box { int *buf; };
int main() {
    struct Box b;
    b.buf = 0;
    *b.buf = 1;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
		},
		{
			Name: "null alias q=p then arrow",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = 0;
    struct Node *q = p;
    q->x = 1;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
		},
		{
			Name: "nullify through int** side-effect then deref",
			Code: `
#include <stdlib.h>
void free2(int **a) {
    free(*a);
    *a = 0;
}
int main() {
    int *p = (int*)malloc(20);
    free2(&p);
    *p = 10;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
		},
		{
			Name: "target filter pa only",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *pa = 0;
    struct Node *pb = 0;
    struct Node *ok = (struct Node*)malloc(sizeof(struct Node));
    pa->x = 11;
    pb->x = 22;
    ok->x = 33;
    return 0;
}
`,
			Rule: `
pa as $pa
pb as $pb
ok as $ok
<npd(target=$pa)> as $npd_pa
<npd(target=$pb)> as $npd_pb
<npd(target=$ok)> as $npd_ok
`,
			Want: map[string]ptrNativeWant{
				"npd_pa": {Min: 1},
				"npd_pb": {Min: 1},
				"npd_ok": {Min: 0, Max: 0},
			},
			Contain: map[string][]string{"npd_pa": {"11"}},
			Absent:  map[string][]string{"npd_pa": {"22"}},
		},
	})
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

// TestC_NPD_Config_Ex — desired behavior for known gaps; soft assertions only.
func TestC_NPD_Config_Ex(t *testing.T) {
	runPtrNativeConfigCases(t, []ptrNativeConfigCase{
		{
			Name: "null then bare star write",
			Code: `
#include <stdlib.h>
int main() {
    int *p = 0;
    *p = 1;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
			Soft: true,
		},
		{
			Name: "if nonnull guard then deref is safe",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main(int c) {
    struct Node *p = 0;
    if (c) { p = (struct Node*)malloc(sizeof(struct Node)); }
    if (p) { p->x = 1; }
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 0, Max: 0}},
			Soft: true,
		},
		{
			Name: "if null guard then deref is npd",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = 0;
    if (!p) { p->x = 1; }
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
			Soft: true,
		},
		{
			Name: "global null then arrow",
			Code: `
#include <stdlib.h>
struct Node { int x; };
struct Node *g;
int main() {
    g = 0;
    g->x = 1;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
			Soft: true,
		},
		{
			Name: "ternary may-null then arrow",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main(int c) {
    struct Node *p = c ? 0 : (struct Node*)malloc(sizeof(struct Node));
    p->x = 1;
    return 0;
}
`,
			Rule: npdScanRule,
			Want: map[string]ptrNativeWant{"npd": {Min: 1}},
			Soft: true,
		},
	})
}
