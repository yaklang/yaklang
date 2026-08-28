package ssaapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// Query natives: heapAlloc, freeCall, derefSite, doubleFree.
const queryNativeRule = `
$focus<heapAlloc()> as $alloc
$focus<freeCall()> as $free
$focus<doubleFree()> as $df
$focus<derefSite()> as $deref
alert $df for { level: "high", risk: "double-free" }
`

// Integration rule: all pointer natives on $focus / $alias.
const allPointerNativesRule = `
$focus<heapAlloc()> as $alloc
$focus<freeCall()> as $free
$focus<doubleFree()> as $df
$focus<uaf()> as $uaf
$focus<npd()> as $npd
$focus<memLeak()> as $leak
$focus<derefSite()> as $deref
$focus<pointsTo()> as $pto
$focus<nullCheck()> as $chk
$alias<aliases(target=$focus)> as $alias_hit

alert $uaf for { level: "high", risk: "use-after-free" }
alert $df for { level: "high", risk: "double-free" }
alert $npd for { level: "high", risk: "null-pointer-deref" }
alert $leak for { level: "low", risk: "memory-leak" }
`

type ptrNativeWant struct {
	Min int // < 0 skips check; 0 with Max 0 means exactly empty
	Max int // 0 means no upper bound
}

type ptrNativeConfigCase struct {
	Name       string
	Code       string
	Rule       string
	Want       map[string]ptrNativeWant
	Contain    map[string][]string
	Absent     map[string][]string
	WantAlerts []string
	Soft       bool
}

func runPtrNativeConfigCase(t *testing.T, tc ptrNativeConfigCase) {
	t.Helper()
	ssatest.CheckWithNameOnlyInMemory("", t, tc.Code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(tc.Rule)
		require.NoError(t, err, tc.Name)
		for varName, want := range tc.Want {
			if want.Min < 0 {
				continue
			}
			got := res.GetValues(varName).Len()
			if tc.Soft {
				if want.Min == 0 && want.Max == 0 && got != 0 {
					t.Logf("gap [%s]: %s should be empty, got %d", tc.Name, varName, got)
					continue
				}
				if want.Min > 0 && got < want.Min {
					t.Logf("gap [%s]: %s min want %d got %d", tc.Name, varName, want.Min, got)
					continue
				}
				if want.Max > 0 && got > want.Max {
					t.Logf("gap [%s]: %s max want %d got %d", tc.Name, varName, want.Max, got)
				}
				continue
			}
			if want.Min == 0 && want.Max == 0 {
				require.Equal(t, 0, got, "%s: %s should be empty", tc.Name, varName)
				continue
			}
			require.GreaterOrEqual(t, got, want.Min, "%s: %s min", tc.Name, varName)
			if want.Max > 0 {
				require.LessOrEqual(t, got, want.Max, "%s: %s max", tc.Name, varName)
			}
		}
		for varName, subs := range tc.Contain {
			if len(subs) == 0 {
				continue
			}
			if tc.Soft {
				if res.GetValues(varName).Len() == 0 {
					t.Logf("gap [%s]: cannot check contain for empty %s", tc.Name, varName)
					continue
				}
				all := res.GetValues(varName).String()
				for _, sub := range subs {
					if !strings.Contains(all, sub) {
						t.Logf("gap [%s]: %s should contain %q", tc.Name, varName, sub)
					}
				}
				continue
			}
			ssatest.CompareResult(t, true, res, map[string][]string{varName: subs})
		}
		for varName, subs := range tc.Absent {
			all := res.GetValues(varName).String()
			for _, s := range subs {
				if tc.Soft && res.GetValues(varName).Len() == 0 {
					continue
				}
				if tc.Soft && strings.Contains(all, s) {
					t.Logf("gap [%s]: %s unexpectedly contains %q", tc.Name, varName, s)
					continue
				}
				require.NotContains(t, all, s, "%s: %s should not contain %q", tc.Name, varName, s)
			}
		}
		if len(tc.WantAlerts) > 0 {
			alerts := res.GetAlertVariables()
			for _, a := range tc.WantAlerts {
				require.Contains(t, alerts, a, "%s: alert %s", tc.Name, a)
			}
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func runPtrNativeConfigCases(t *testing.T, cases []ptrNativeConfigCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			runPtrNativeConfigCase(t, tc)
		})
	}
}

// lifetime_query_syntaxflow_test.go — query natives category.
func TestC_LifetimeQuery_Config(t *testing.T) {
	runPtrNativeConfigCases(t, []ptrNativeConfigCase{
		{
			Name: "int double-free query natives",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    *p = 1;
    free(p);
    free(p);
    return 0;
}
`,
			Rule: `
p as $focus
` + queryNativeRule,
			Want: map[string]ptrNativeWant{
				"alloc": {Min: 1},
				"free":  {Min: 1},
				"df":    {Min: 1},
			},
			WantAlerts: []string{"df"},
		},
		{
			Name: "int single free safe",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    return 0;
}
`,
			Rule: `
p as $focus
` + queryNativeRule,
			Want: map[string]ptrNativeWant{
				"alloc": {Min: 1},
				"free":  {Min: 1},
				"df":    {Min: 0, Max: 0},
			},
		},
		{
			Name: "int alias double-free q=p",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int *q = p;
    free(p);
    free(q);
    return 0;
}
`,
			Rule: `
p as $focus
` + queryNativeRule,
			Want: map[string]ptrNativeWant{
				"df":   {Min: 1},
				"free": {Min: 1},
			},
			WantAlerts: []string{"df"},
		},
		{
			Name: "struct pointer double-free",
			Code: `
#include <stdlib.h>
struct Box { int v; };
int main() {
    struct Box *p = (struct Box*)malloc(sizeof(struct Box));
    free(p);
    free(p);
    return 0;
}
`,
			Rule: `
p as $focus
` + queryNativeRule,
			Want: map[string]ptrNativeWant{
				"alloc": {Min: 1},
				"df":    {Min: 1},
			},
		},
		{
			Name: "struct nested field freeCall on inner",
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
			Rule: `
w as $focus
` + queryNativeRule + `
*<heapAlloc()> as $all_alloc
*<freeCall()> as $free_all
`,
			Want: map[string]ptrNativeWant{
				"alloc":     {Min: 1},
				"all_alloc": {Min: 2},
				"free_all":  {Min: 2},
			},
		},
		{
			Name: "multilevel int** deref after free",
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
			Rule: `
p as $focus
` + queryNativeRule,
			Want: map[string]ptrNativeWant{
				"alloc": {Min: 1},
				"free":  {Min: 1},
				"deref": {Min: 1},
			},
		},
		{
			Name: "star target filter pa vs pb",
			Code: `
#include <stdlib.h>
int main() {
    int *pa = (int*)malloc(sizeof(int));
    int *pb = (int*)malloc(sizeof(int));
    free(pa);
    free(pa);
    free(pb);
    return 0;
}
`,
			Rule: `
pa as $pa
pb as $pb
*<doubleFree(target=$pa)> as $df_pa
*<doubleFree(target=$pb)> as $df_pb
*<heapAlloc(target=$pa)> as $alloc_pa
*<freeCall(target=$pa)> as $free_pa
*<doubleFree(target=$missing)> as $df_missing
`,
			Want: map[string]ptrNativeWant{
				"df_pa":      {Min: 1},
				"df_pb":      {Min: 0, Max: 0},
				"alloc_pa":   {Min: 1},
				"free_pa":    {Min: 1},
				"df_missing": {Min: 0, Max: 0},
			},
		},
		{
			Name: "receiver chain heapAlloc freeCall",
			Code: `
#include <stdlib.h>
int main() {
    int *pa = (int*)malloc(sizeof(int));
    int *pb = (int*)malloc(sizeof(int));
    free(pa);
    free(pa);
    free(pb);
    return 0;
}
`,
			Rule: `
pa as $pa
$pa<heapAlloc()> as $alloc
$pa<freeCall()> as $free
$pa<doubleFree()> as $df
`,
			Want: map[string]ptrNativeWant{
				"alloc": {Min: 1},
				"free":  {Min: 1},
				"df":    {Min: 1},
			},
		},
		{
			Name: "derefSite target filter",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    int x = *p;
    free(p);
    return x;
}
`,
			Rule: `
p as $p
*<derefSite()> as $d_all
<derefSite(target=$p)> as $d_tgt
`,
			Want: map[string]ptrNativeWant{
				"d_all": {Min: 1},
				"d_tgt": {Min: 1},
			},
		},
	})
}

func TestC_LifetimeQuery_AlertConfig(t *testing.T) {
	runPtrNativeConfigCase(t, ptrNativeConfigCase{
		Name: "alert double-free with target",
		Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    free(p);
    return 0;
}
`,
		Rule: `
p as $focus
` + queryNativeRule + `
alert $df for { level: "high", risk: "double-free" }
`,
		Want: map[string]ptrNativeWant{
			"df":    {Min: 1},
			"alloc": {Min: 1},
		},
		WantAlerts: []string{"df"},
	})
}

func TestC_LifetimeQuery_LifetimeAPI(t *testing.T) {
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
		require.Greater(t, len(lifetime.ListHeapAllocs(prog.Program)), 0)
		require.Greater(t, len(lifetime.ListFreeCalls(prog.Program)), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

// TestC_PointerNative_All_Config — integration: all pointer natives in one config rule.
func TestC_PointerNative_All_Config(t *testing.T) {
	runPtrNativeConfigCases(t, []ptrNativeConfigCase{
		{
			Name: "dashboard leak + double-free",
			Code: `
#include <stdlib.h>
int main() {
    int *leak = (int*)malloc(sizeof(int));
    int *df = (int*)malloc(sizeof(int));
    free(df);
    free(df);
    return 0;
}
`,
			Rule: `
*<heapAlloc()> as $alloc
*<freeCall()> as $free
*<doubleFree()> as $df
*<memLeak()> as $leak
*<uaf()> as $uaf
alert $df for { level: "high", risk: "double-free" }
alert $leak for { level: "low", risk: "memory-leak" }
`,
			Want: map[string]ptrNativeWant{
				"alloc": {Min: 2},
				"free":  {Min: 2},
				"df":    {Min: 1},
				"leak":  {Min: 1},
				"uaf":   {Min: 1},
			},
			WantAlerts: []string{"df", "leak"},
		},
		{
			Name: "struct + multilevel all natives on focus",
			Code: `
#include <stdlib.h>
struct Node { int v; struct Node *next; };
int main() {
    struct Node *p = (struct Node*)malloc(sizeof(struct Node));
    struct Node *q = p;
    int **slot = &p;
    p->v = 1;
    free(p);
    q->v = 2;
    **slot = 3;
    return 0;
}
`,
			Rule: `
p as $focus
q as $alias
` + allPointerNativesRule,
			Want: map[string]ptrNativeWant{
				"alloc":     {Min: 1},
				"free":      {Min: 1},
				"uaf":       {Min: 1},
				"alias_hit": {Min: 1},
				"pto":       {Min: 1},
			},
			WantAlerts: []string{"uaf"},
		},
		{
			Name: "uaf kind filter",
			Code: `
#include <stdlib.h>
int main() {
    int *p = (int*)malloc(sizeof(int));
    free(p);
    free(p);
    return 0;
}
`,
			Rule: `
*<uaf(kind="double-free")> as $df
*<doubleFree()> as $df2
`,
			Want: map[string]ptrNativeWant{
				"df":  {Min: 1},
				"df2": {Min: 1},
			},
		},
	})
}
