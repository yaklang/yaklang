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

	t.Run("exponential tree keeps real zero values on the bounded prefix", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("package main\n")
		b.WriteString("type L0 struct{ A int; B int }\n")
		for i := 1; i <= 20; i++ {
			fmt.Fprintf(&b, "type L%d struct{ X L%d; Y L%d }\n", i, i-1, i-1)
		}
		chain := "v" + strings.Repeat(".X", 20) + ".A"
		b.WriteString("func main() { var v L20; println(" + chain + ") }")
		// the leftmost chain is built first and stays within the node limit,
		// so it keeps the real zero value instead of a placeholder
		test.CheckPrintlnValueContain(b.String(), []string{"0"}, t)
	})

	t.Run("exponential tree degrades past the node limit", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("package main\n")
		b.WriteString("type L0 struct{ A int; B int }\n")
		for i := 1; i <= 20; i++ {
			fmt.Fprintf(&b, "type L%d struct{ X L%d; Y L%d }\n", i, i-1, i-1)
		}
		chain := "v" + strings.Repeat(".Y", 20) + ".A"
		b.WriteString("func main() { var v L20; println(" + chain + ") }")
		// the all-Y path is built after the left subtree has exhausted the
		// node limit, so it degrades to a placeholder instead of a zero value
		test.CheckPrintlnValueContain(b.String(), []string{"Undefined"}, t)
	})

	t.Run("invalid member access on exponential type reports a bounded error", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("package main\n")
		b.WriteString("type L0 struct{ A int; B int }\n")
		for i := 1; i <= 20; i++ {
			fmt.Fprintf(&b, "type L%d struct{ X L%d; Y L%d }\n", i, i-1, i-1)
		}
		b.WriteString("func main() { var v L20; println(v.NoSuchField) }")
		test.CheckPrintlnValueContain(b.String(), []string{"Undefined"}, t)
	})
}
