package ssaapi

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func buildTopDefPerfCode(depth int) string {
	var sb strings.Builder
	sb.WriteString(`seed = "topdef-seed"` + "\n")
	sb.WriteString(`mk0 = (x) => { return {"v": x} }` + "\n")
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&sb, "mk%d = (x) => { o = mk%d(x); return {\"v\": o.v + \"_%d\"} }\n", i, i-1, i)
	}
	fmt.Fprintf(&sb, "sinkObj = mk%d(seed)\n", depth)
	sb.WriteString("sink = sinkObj.v\n")
	return sb.String()
}

func runTopDefOnce(t testing.TB, prog *ssaapi.Program) ssaapi.Values {
	t.Helper()
	refs := prog.Ref("sink")
	if len(refs) == 0 {
		t.Fatalf("cannot find sink variable")
	}
	return refs.Get(0).GetTopDefs()
}

func topDefFingerprint(vals ssaapi.Values) string {
	items := make([]string, 0, len(vals))
	for _, v := range vals {
		items = append(items, v.String())
	}
	sort.Strings(items)
	return strings.Join(items, "|")
}

func TestTopDefPerfRegression(t *testing.T) {
	code := buildTopDefPerfCode(80)
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.Yak))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	start := time.Now()
	base := runTopDefOnce(t, prog)
	firstCost := time.Since(start)
	if len(base) == 0 {
		t.Fatalf("topdef is empty")
	}
	baseFP := topDefFingerprint(base)

	for i := 0; i < 5; i++ {
		got := runTopDefOnce(t, prog)
		if len(got) == 0 {
			t.Fatalf("topdef is empty at round %d", i+1)
		}
		if gotFP := topDefFingerprint(got); gotFP != baseFP {
			t.Fatalf("round %d: topdef result changed unexpectedly", i+1)
		}
	}
	t.Logf("topdef smoke cost=%s, size=%d", firstCost, len(base))
}

func BenchmarkTopDefPerf(b *testing.B) {
	code := buildTopDefPerfCode(80)
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.Yak))
	if err != nil {
		b.Fatalf("parse failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got := runTopDefOnce(b, prog)
		if len(got) == 0 {
			b.Fatalf("invalid topdef result at round %d", i)
		}
	}
}
