package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	ssatest "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func adversarialSelectSSA(count int) string {
	var code strings.Builder
	code.WriteString("ch=make(chan int); selected=-1; select {")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&code, "case <-ch: selected=%d;\n", i)
	}
	code.WriteString("}; println(selected)")
	return code.String()
}

func TestSelectAdversarialSSA(t *testing.T) {
	ssatest.CheckNoError(t, adversarialSelectSSA(256))
	ssatest.CheckNoError(t, strings.Repeat("select {default:", 64)+"println(7)"+strings.Repeat("}", 64))
	p, err := ssaapi.Parse(adversarialSelectSSA(64), ssaapi.WithEnableCache(false))
	require.NoError(t, err)
	values := p.Ref("selected")
	require.NotEmpty(t, values)
	require.Contains(t, values.String(), "63")
}

func TestSelectAdversarialSSAExactJoin(t *testing.T) {
	var values []string
	for i := 0; i < 64; i++ {
		values = append(values, fmt.Sprint(i))
	}
	ssatest.CheckPrintlnValue(adversarialSelectSSA(64), []string{"phi(selected)[" + strings.Join(values, ",") + "]"}, t)
}

func BenchmarkSelectAdversarialSSA(b *testing.B) {
	for _, count := range []int{16, 128, 512} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			code := adversarialSelectSSA(count)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := ssaapi.Parse(code, ssaapi.WithEnableCache(false)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
