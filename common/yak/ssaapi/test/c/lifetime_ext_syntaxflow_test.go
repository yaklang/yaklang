package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// Ext natives: memLeak, nullCheck, pointsTo, aliases.
const extNativeRule = `
$focus<memLeak()> as $leak
$focus<nullCheck()> as $chk
$focus<pointsTo()> as $pto
$alias<aliases(target=$focus)> as $alias_hit
alert $leak for { level: "low", risk: "memory-leak" }
`

const memLeakScanRule = `*<memLeak()> as $leak`
const nullCheckScanRule = `*<nullCheck()> as $chk`

func TestC_MemLeak_Config(t *testing.T) {
	runPtrNativeConfigCases(t, []ptrNativeConfigCase{
		{
			Name: "basic malloc never freed",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 1;
    return 0;
}
`,
			Rule: `p as $focus` + extNativeRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 1}},
			WantAlerts: []string{"leak"},
		},
		{
			Name: "calloc never freed",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)calloc(1, sizeof(int));
    return 0;
}
`,
			Rule: `p as $focus` + extNativeRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 1}},
		},
		{
			Name: "free then return is safe",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    return 0;
}
`,
			Rule: `p as $focus` + extNativeRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 0, Max: 0}},
		},
		{
			Name: "if may-free still may leak",
			Code: `
#include <stdlib.h>
int main(int abrt) {
    int *p = (int*)malloc(sizeof(int));
    if (abrt) {
        free(p);
    }
    return 0;
}
`,
			Rule: `p as $focus` + extNativeRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 1}},
		},
		{
			Name: "free in both branches is safe",
			Code: `
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
			Rule: `p as $focus` + extNativeRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 0, Max: 0}},
		},
		{
			Name: "return ownership escape is safe",
			Code: `
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
			Rule: `q as $focus` + extNativeRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 0, Max: 0}},
		},
		{
			Name: "two allocs one leaked",
			Code: `
#include <stdlib.h>
int main() {
    int *pa = (int*)malloc(sizeof(int));
    int *pb = (int*)malloc(sizeof(int));
    free(pb);
    return 0;
}
`,
			Rule: memLeakScanRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 1}},
		},
		{
			Name: "struct nested field mem leak",
			Code: `
#include <stdlib.h>
struct Wrapper { int *data; };
int main() {
    struct Wrapper *w = (struct Wrapper*)malloc(sizeof(struct Wrapper));
    w->data = (int*)malloc(sizeof(int));
    free(w);
    return 0;
}
`,
			Rule: memLeakScanRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 1}},
		},
		{
			Name: "multilevel int** heap leak",
			Code: `
#include <stdlib.h>
int main() {
    int **pp = (int**)malloc(sizeof(int*));
    *pp = (int*)malloc(sizeof(int));
    return 0;
}
`,
			Rule: `
pp as $focus
` + extNativeRule + `
*pp as $inner
$inner<memLeak()> as $inner_leak
`,
			Want: map[string]ptrNativeWant{
				"leak":       {Min: 1},
				"inner_leak": {Min: 1},
			},
		},
		{
			Name: "target filter pa leaks pb freed",
			Code: `
#include <stdlib.h>
int main() {
    int *pa = (int*)malloc(sizeof(int));
    int *pb = (int*)malloc(sizeof(int));
    int *safe = (int*)malloc(sizeof(int));
    free(safe);
    free(pb);
    return 0;
}
`,
			Rule: `
pa as $pa
pb as $pb
safe as $safe
<memLeak(target=$pa)> as $leak_pa
*<memLeak(target=$pa)> as $leak_pa_star
$pa<memLeak()> as $leak_pa_recv
<memLeak(target=$pb)> as $leak_pb
<memLeak(target=$safe)> as $leak_safe
`,
			Want: map[string]ptrNativeWant{
				"leak_pa":      {Min: 1},
				"leak_pa_star": {Min: 1},
				"leak_pa_recv": {Min: 1},
				"leak_pb":      {Min: 0, Max: 0},
				"leak_safe":    {Min: 0, Max: 0},
			},
		},
	})
}

func TestC_NullCheck_Config(t *testing.T) {
	runPtrNativeConfigCases(t, []ptrNativeConfigCase{
		{
			Name: "bare if (p)",
			Code: `
#include <stdlib.h>
int main(int *p) {
    if (p) { *p = 1; }
    return 0;
}
`,
			Rule: nullCheckScanRule,
			Want: map[string]ptrNativeWant{"chk": {Min: 1}},
		},
		{
			Name: "if (p != 0)",
			Code: `
#include <stdlib.h>
int main(int *p) {
    if (p != 0) { *p = 1; }
    return 0;
}
`,
			Rule: nullCheckScanRule,
			Want: map[string]ptrNativeWant{"chk": {Min: 1}},
		},
		{
			Name: "if (p == 0)",
			Code: `
#include <stdlib.h>
int main(int *p) {
    if (p == 0) { return 1; }
    *p = 1;
    return 0;
}
`,
			Rule: nullCheckScanRule,
			Want: map[string]ptrNativeWant{"chk": {Min: 1}},
		},
		{
			Name: "if (!p)",
			Code: `
#include <stdlib.h>
int main(int *p) {
    if (!p) { return 1; }
    *p = 1;
    return 0;
}
`,
			Rule: nullCheckScanRule,
			Want: map[string]ptrNativeWant{"chk": {Min: 1}},
		},
		{
			Name: "no null check",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 1;
    free(p);
    return 0;
}
`,
			Rule: nullCheckScanRule,
			Want: map[string]ptrNativeWant{"chk": {Min: 0, Max: 0}},
		},
		{
			Name: "struct param nullCheck",
			Code: `
#include <stdlib.h>
struct Node { int x; };
int main(struct Node *p) {
    if (p != 0) { p->x = 1; }
    return 0;
}
`,
			Rule: `p as $focus` + extNativeRule,
			Want: map[string]ptrNativeWant{"chk": {Min: 1}},
		},
		{
			Name: "target filter pa and pb",
			Code: `
#include <stdlib.h>
int main(int *pa, int *pb) {
    if (pa) { *pa = 11; }
    if (pb == 0) { return 1; }
    *pb = 22;
    return 0;
}
`,
			Rule: `
pa as $pa
pb as $pb
<nullCheck(target=$pa)> as $chk_pa
$pb<nullCheck()> as $chk_pb
*<nullCheck()> as $chk_all
`,
			Want: map[string]ptrNativeWant{
				"chk_pa":  {Min: 1},
				"chk_pb":  {Min: 1},
				"chk_all": {Min: 2},
			},
		},
	})
}

func TestC_PointsTo_Aliases_Config(t *testing.T) {
	runPtrNativeConfigCases(t, []ptrNativeConfigCase{
		{
			Name: "int alias q=p pointsTo",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    int *r = (int*)malloc(sizeof(int));
    free(p);
    free(r);
    return 0;
}
`,
			Rule: `
p as $focus
q as $alias
r as $other
` + extNativeRule + `
$other<aliases(target=$focus)> as $other_alias
`,
			Want: map[string]ptrNativeWant{
				"pto":         {Min: 1},
				"alias_hit":   {Min: 1},
				"other_alias": {Min: 0, Max: 0},
			},
		},
		{
			Name: "struct alias chain",
			Code: `
#include <stdlib.h>
struct Node { int v; struct Node *next; };
int main() {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    struct Node *q = p;
    struct Node *r = (struct Node*)malloc(sizeof(struct Node));
    p->v = 1;
    free(p);
    free(r);
    return 0;
}
`,
			Rule: `
p as $focus
q as $alias
r as $other
` + extNativeRule + `
$other<aliases(target=$focus)> as $other_alias
`,
			Want: map[string]ptrNativeWant{
				"alias_hit":   {Min: 1},
				"other_alias": {Min: 0, Max: 0},
				"pto":         {Min: 1},
			},
		},
		{
			Name: "multilevel slot alias",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int **slot = &p;
    int *q = *slot;
    free(p);
    return 0;
}
`,
			Rule: `
p as $focus
q as $alias
` + extNativeRule,
			Want: map[string]ptrNativeWant{
				"alias_hit": {Min: 1},
				"pto":       {Min: 1},
			},
		},
	})
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

// TestC_MemLeak_Config_Ex — soft assertions for known analysis gaps.
func TestC_MemLeak_Config_Ex(t *testing.T) {
	runPtrNativeConfigCases(t, []ptrNativeConfigCase{
		{
			Name: "nested free wrapper then no leak",
			Code: `
#include <stdlib.h>
void freep(int *p) { free(p); }
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep(p);
    return 0;
}
`,
			Rule: memLeakScanRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 0, Max: 0}},
			Soft: true,
		},
		{
			Name: "early return without free is leak",
			Code: `
#include <stdlib.h>
int main(int c) {
    int *p = (int*)malloc(sizeof(int));
    if (c) { return 1; }
    free(p);
    return 0;
}
`,
			Rule: memLeakScanRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 1}},
			Soft: true,
		},
		{
			Name: "alloc in loop body freed is safe",
			Code: `
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
			Rule: memLeakScanRule,
			Want: map[string]ptrNativeWant{"leak": {Min: 0, Max: 0}},
			Soft: true,
		},
	})
}
