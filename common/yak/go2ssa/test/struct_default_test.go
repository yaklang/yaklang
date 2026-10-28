package test

import (
	"fmt"
	"strings"
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestStructDefaultExpansionPruned guards GetDefaultValue against exponential
// default-value trees. A type graph where every level holds two fields of the
// previous level expands to 2^N nodes (no dedup), which used to make the
// compiler never finish building `var v L20`. Types whose expansion exceeds
// the node/depth limits degrade to a typed Make placeholder inside
// GetDefaultValue, so deeper reads resolve lazily while small structs
// keep exact zero values.
func TestStructDefaultExpansionPruned(t *testing.T) {
	t.Run("small struct keeps exact zero-value members", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		type Leaf struct {
			N int
			M int
		}
		type Small struct {
			A Leaf
			B Leaf
		}
		func main() {
			var s Small
			println(s.A.N)
			println(s.B.M)
		}
		`, []string{"0", "0"}, t)
	})

	t.Run("deep linear nesting degrades past the depth limit", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("package main\n")
		b.WriteString("type L0 struct{ A int }\n")
		for i := 1; i <= 80; i++ {
			fmt.Fprintf(&b, "type L%d struct{ F L%d; A int }\n", i, i-1)
		}
		shallow := "v" + strings.Repeat(".F", 63) + ".A"
		deep := "v" + strings.Repeat(".F", 75) + ".A"
		b.WriteString("func main() { var v L80; println(" + shallow + "); println(" + deep + ") }")
		// 63 nesting levels stay within defaultValueDepthLimit and keep the
		// real zero; 75 levels degrade to a placeholder
		test.CheckPrintlnValueContain(b.String(), []string{"0", "Undefined"}, t)
	})

	t.Run("exponential tree degrades past the node limit", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("package main\n")
		b.WriteString("type L0 struct{ A int; B int }\n")
		for i := 1; i <= 10; i++ {
			fmt.Fprintf(&b, "type L%d struct{ X L%d; Y L%d }\n", i, i-1, i-1)
		}
		left := "v" + strings.Repeat(".X", 10) + ".A"
		right := "v" + strings.Repeat(".Y", 10) + ".A"
		b.WriteString("func main() { var v L10; println(" + left + "); println(" + right + ") }")
		// the raw field tree holds 2^11-1 = 2047 struct nodes, past the
		// 1024-node budget: the leftmost chain is built first and keeps the
		// real zero value, the all-Y path degrades to a placeholder
		test.CheckPrintlnValueContain(b.String(), []string{"0", "Undefined"}, t)
	})

	t.Run("invalid member access on exponential type reports a bounded error", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("package main\n")
		b.WriteString("type L0 struct{ A int; B int }\n")
		for i := 1; i <= 10; i++ {
			fmt.Fprintf(&b, "type L%d struct{ X L%d; Y L%d }\n", i, i-1, i-1)
		}
		b.WriteString("func main() { var v L10; println(v.NoSuchField) }")
		test.CheckPrintlnValueContain(b.String(), []string{"Undefined"}, t)
	})
}
