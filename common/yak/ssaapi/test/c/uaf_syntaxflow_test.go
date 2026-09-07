package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// UAF / doubleFree native category.
const uafNativeRule = `
$focus<uaf()> as $uaf
$focus<doubleFree()> as $df
alert $uaf for { level: "high", risk: "use-after-free" }
alert $df for { level: "high", risk: "double-free" }
`

const uafScanRule = `*<uaf()> as $uaf`

func uafScanCase(name, code string, wantUAF bool, contain ...string) ptrNativeConfigCase {
	want := map[string]ptrNativeWant{"uaf": {Min: 0, Max: 0}}
	if wantUAF {
		want["uaf"] = ptrNativeWant{Min: 1}
	}
	c := map[string][]string{}
	if len(contain) > 0 {
		c["uaf"] = contain
	}
	return ptrNativeConfigCase{Name: name, Code: code, Rule: uafScanRule, Want: want, Contain: c}
}

func TestC_UAF_Config(t *testing.T) {
	runPtrNativeConfigCases(t, append([]ptrNativeConfigCase{
		{
			Name: "struct member UAF after free",
			Code: `
#include <stdlib.h>
struct Node { int x; int y; };
int main() {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    p->x = 1;
    free(p);
    p->y = 2;
    return 0;
}
`,
			Rule: `p as $focus` + uafNativeRule,
			Want: map[string]ptrNativeWant{"uaf": {Min: 1}},
			WantAlerts: []string{"uaf"},
		},
		{
			Name: "struct nested int* field UAF",
			Code: `
#include <stdlib.h>
struct Wrapper { int *data; };
int main() {
    struct Wrapper *w = (struct Wrapper*)malloc(sizeof(struct Wrapper));
    w->data = (int*)malloc(sizeof(int));
    free(w->data);
    *w->data = 99;
    free(w);
    return 0;
}
`,
			Rule: uafScanRule,
			Want: map[string]ptrNativeWant{"uaf": {Min: 1}},
		},
		{
			Name: "multilevel int** UAF via slot",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int **pp = &p;
    *p = 1;
    free(p);
    **pp = 2;
    return 0;
}
`,
			Rule: `p as $focus` + uafNativeRule,
			Want: map[string]ptrNativeWant{"uaf": {Min: 1}},
			Contain: map[string][]string{"uaf": {"2"}},
		},
		{
			Name: "struct linked alias UAF",
			Code: `
#include <stdlib.h>
struct Node { int v; struct Node *next; };
int main() {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    struct Node *q = p;
    p->v = 1;
    free(p);
    q->v = 2;
    return 0;
}
`,
			Rule: `
p as $focus
q as $alias
` + uafNativeRule + `
$alias<uaf()> as $uaf_alias
`,
			Want: map[string]ptrNativeWant{
				"uaf":       {Min: 1},
				"uaf_alias": {Min: 1},
			},
		},
		{
			Name: "target filter pa only",
			Code: `
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
`,
			Rule: `
pa as $pa
pb as $pb
safe as $safe
<uaf(target=$pa)> as $uaf_pa
*<uaf(target=$pa)> as $uaf_pa_star
$pa<uaf()> as $uaf_pa_recv
<uaf(target=$pb)> as $uaf_pb
<uaf(target=$safe)> as $uaf_safe
`,
			Want: map[string]ptrNativeWant{
				"uaf_pa":      {Min: 1},
				"uaf_pa_star": {Min: 1},
				"uaf_pa_recv": {Min: 1},
				"uaf_pb":      {Min: 1},
				"uaf_safe":    {Min: 0, Max: 0},
			},
			Contain: map[string][]string{"uaf_pa": {"11"}},
			Absent:  map[string][]string{"uaf_pa": {"22"}},
		},
		{
			Name: "target alias q=p after free",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    free(p);
    *q = 3;
    return 0;
}
`,
			Rule: `
q as $focus
` + uafNativeRule,
			Want: map[string]ptrNativeWant{"uaf": {Min: 1}},
			Contain: map[string][]string{"uaf": {"3"}},
		},
		{
			Name: "alert config",
			Code: `
#include <stdlib.h>
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    free(ptr);
    *ptr = 20;
    return 0;
}
`,
			Rule: uafScanRule + `
alert $uaf for { level: "high", risk: "Use After Free" }
`,
			Want: map[string]ptrNativeWant{"uaf": {Min: 1}},
			WantAlerts: []string{"uaf"},
		},
		{
			Name: "free? filter outputs the free call not the use site",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 10;
    free(p);
    *p = 20;
    return 0;
}
`,
			Rule: `free?(* #-> <uaf()>) as $uaf`,
			Want: map[string]ptrNativeWant{"uaf": {Min: 1}},
			Contain: map[string][]string{"uaf": {"free"}},
			Absent:  map[string][]string{"uaf": {"20"}},
		},
		{
			Name: "free? filter keeps only the free whose pointer has UAF",
			Code: `
#include <stdlib.h>
int main() {
    int *pa = (int*)malloc(sizeof(int));
    int *pb = (int*)malloc(sizeof(int));
    *pb = 0;
    free(pa);
    *pa = 11;
    free(pb);
    return 0;
}
`,
			Rule: `free?(* #-> <uaf()>) as $uaf`,
			Want: map[string]ptrNativeWant{"uaf": {Min: 1, Max: 1}},
			Contain: map[string][]string{"uaf": {"free"}},
			Absent:  map[string][]string{"uaf": {"11"}},
		},
		{
			Name: "free? filter empty when no UAF",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 10;
    free(p);
    return 0;
}
`,
			Rule: `free?(* #-> <uaf()>) as $uaf`,
			Want: map[string]ptrNativeWant{"uaf": {Min: 0, Max: 0}},
		},
	}, uafCoreCases()...))
}

func uafCoreCases() []ptrNativeConfigCase {
	return []ptrNativeConfigCase{
		uafScanCase("basic free then deref write", `
#include <stdlib.h>
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    *ptr = 10;
    free(ptr);
    *ptr = 20;
    return 0;
}
`, true, "20"),
		uafScanCase("arrow member after free", `
#include <stdlib.h>
struct Node { int x; };
int main() {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    free(p);
    p->x = 1;
    return 0;
}
`, true, "1"),
		uafScanCase("copy alias q=p", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    free(p);
    *q = 3;
    return 0;
}
`, true, "3"),
		uafScanCase("if may-free then use after join", `
#include <stdlib.h>
int main(int abrt) {
    int *ptr = (int*)malloc(sizeof(int));
    *ptr = 10;
    if (abrt) { free(ptr); }
    *ptr = 20;
    return 0;
}
`, true, "20"),
		uafScanCase("if-else free in then use after join", `
#include <stdlib.h>
int main(int cond) {
    int *p = (int*)malloc(sizeof(int));
    if (cond) { free(p); } else { *p = 1; }
    *p = 2;
    return 0;
}
`, true, "2"),
		uafScanCase("if free in both branches then use", `
#include <stdlib.h>
int main(int cond) {
    int *p = (int*)malloc(sizeof(int));
    if (cond) { free(p); } else { free(p); }
    *p = 9;
    return 0;
}
`, true, "9"),
		uafScanCase("if free in then use only in else is safe", `
#include <stdlib.h>
int main(int cond) {
    int *p = (int*)malloc(sizeof(int));
    if (cond) { free(p); } else { *p = 1; free(p); }
    return 0;
}
`, false),
		uafScanCase("for free then use after loop", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int i;
    for (i = 0; i < 1; i++) { free(p); }
    *p = 7;
    return 0;
}
`, true),
		uafScanCase("for alloc use free inside body is safe", `
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
`, false),
		uafScanCase("while free then use after loop", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int i = 0;
    while (i < 1) { free(p); i++; }
    *p = 6;
    return 0;
}
`, true),
		uafScanCase("for free then use in later iteration", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int i;
    for (i = 0; i < 2; i++) {
        if (i == 0) { free(p); } else { *p = 5; }
    }
    return 0;
}
`, true),
		uafScanCase("if inside for free then use after for", `
#include <stdlib.h>
int main(int flag) {
    int *p = (int*)malloc(sizeof(int));
    int i;
    for (i = 0; i < 1; i++) { if (flag) { free(p); } }
    *p = 3;
    return 0;
}
`, true, "3"),
		uafScanCase("cross-func freep then use in caller", `
#include <stdlib.h>
void freep(int *p) { free(p); }
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    freep(ptr);
    *ptr = 20;
    return 0;
}
`, true, "20"),
		uafScanCase("cross-func free then pass to user", `
#include <stdlib.h>
void touch(int *p) { *p = 4; }
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    free(ptr);
    touch(ptr);
    return 0;
}
`, true),
		uafScanCase("cross-func wrapper free with alias", `
#include <stdlib.h>
void freep(int *p) { free(p); }
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    freep(p);
    *q = 8;
    return 0;
}
`, true, "8"),
		uafScanCase("cross-func freep under if then use", `
#include <stdlib.h>
void freep(int *p) { free(p); }
int main(int cond) {
    int *p = (int*)malloc(sizeof(int));
    if (cond) { freep(p); }
    *p = 11;
    return 0;
}
`, true, "11"),
		uafScanCase("safe use before free", `
#include <stdlib.h>
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    *ptr = 10;
    free(ptr);
    return 0;
}
`, false),
		uafScanCase("safe null after free", `
#include <stdlib.h>
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    free(ptr);
    ptr = 0;
    return 0;
}
`, false),
		uafScanCase("unrelated pointer not uaf", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *r = (int*)malloc(sizeof(int));
    free(p);
    *r = 1;
    free(r);
    return 0;
}
`, false),
		uafScanCase("cross-func safe freep without later use", `
#include <stdlib.h>
void freep(int *p) { free(p); }
int main() {
    int *ptr = (int*)malloc(sizeof(int));
    *ptr = 1;
    freep(ptr);
    return 0;
}
`, false),
		uafScanCase("param free then call use", `
#include <stdlib.h>
void sink(int *q);
void f(int *p) { free(p); sink(p); }
`, true),
		uafScanCase("param free then arrow member write", `
#include <stdlib.h>
struct Node { int x; };
void f(struct Node *p) { free(p); p->x = 1; }
`, true, "1"),
		uafScanCase("param may-free then call use after join", `
#include <stdlib.h>
void sink(int *q);
void f(int *p, int c) { if (c) { free(p); } sink(p); }
`, true),
		uafScanCase("param use then free is safe", `
#include <stdlib.h>
void sink(int *q);
void f(int *p) { sink(p); free(p); }
`, false),
		uafScanCase("param free without later use is safe", `
#include <stdlib.h>
void f(int *p) { free(p); }
`, false),
		uafScanCase("double free via int** wrapper", `
#include <stdlib.h>
void free2(int **a) { free(*a); }
int main() {
    int *p = (int*)malloc(20);
    free2(&p);
    free2(&p);
    return 0;
}
`, true),
		uafScanCase("double free basic", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    free(p);
    return 0;
}
`, true),
		uafScanCase("double free via alias q=p", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    free(p);
    free(q);
    return 0;
}
`, true),
		uafScanCase("double free freep then free", `
#include <stdlib.h>
void freep(int *p) { free(p); }
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep(p);
    free(p);
    return 0;
}
`, true),
		uafScanCase("double free on formal parameter", `
#include <stdlib.h>
void f(int *p) { free(p); free(p); }
`, true),
		uafScanCase("may-free then free is double free", `
#include <stdlib.h>
int main(int abrt) {
    int *p = (int*)malloc(sizeof(int));
    if (abrt) { free(p); }
    free(p);
    return 0;
}
`, true),
		uafScanCase("free once is safe not double free", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 1;
    free(p);
    return 0;
}
`, false),
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

// TestC_UAF_Config_Ex — soft assertions for known analysis gaps.
func TestC_UAF_Config_Ex(t *testing.T) {
	cases := []ptrNativeConfigCase{
		uafScanCase("nested wrapper freep2 then use", `
#include <stdlib.h>
void freep(int *p) { free(p); }
void freep2(int *p) { freep(p); }
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep2(p);
    *p = 7;
    return 0;
}
`, true, "7"),
		uafScanCase("nested wrapper freep2 then free is double free", `
#include <stdlib.h>
void freep(int *p) { free(p); }
void freep2(int *p) { freep(p); }
int main() {
    int *p = (int*)malloc(sizeof(int));
    freep2(p);
    free(p);
    return 0;
}
`, true),
		uafScanCase("null after free then deref is safe", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    p = 0;
    *p = 1;
    return 0;
}
`, false),
		uafScanCase("global free then use", `
#include <stdlib.h>
int *g;
int main() {
    g = (int*)malloc(sizeof(int));
    free(g);
    *g = 3;
    return 0;
}
`, true, "3"),
		uafScanCase("param free then bare star write", `
#include <stdlib.h>
void f(int *p) { free(p); *p = 9; }
`, true, "9"),
		uafScanCase("struct field free then use", `
#include <stdlib.h>
struct Box { int *buf; };
int main() {
    struct Box b;
    b.buf = (int*)malloc(sizeof(int));
    free(b.buf);
    *b.buf = 5;
    return 0;
}
`, true, "5"),
		uafScanCase("use old pointer after realloc move", `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(8);
    int *q = (int*)realloc(p, 16);
    *p = 6;
    free(q);
    return 0;
}
`, true, "6"),
		uafScanCase("store freed ptr then load and use", `
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
`, true, "8"),
		uafScanCase("free then pass to store-only sink is safe target", `
#include <stdlib.h>
static int *held;
void hold(int *p) { held = p; }
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    hold(p);
    return 0;
}
`, false),
		uafScanCase("triple wrapper freep3 then use", `
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
`, true, "2"),
		uafScanCase("wrapper frees local alias of param", `
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
`, true, "11"),
	}
	for i := range cases {
		cases[i].Soft = true
	}
	runPtrNativeConfigCases(t, cases)
}
