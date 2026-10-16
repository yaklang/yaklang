package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// A named pointer field must not eagerly materialize its target's zero value.
// ARTEX has mutually recursive pointer fields; erasing * turned that finite
// initialization into an unbounded GetDefaultValue -> InterfaceAddFieldBuild.
func TestGoZeroValueNamedPointerFields(t *testing.T) {
	prog, err := ssaapi.Parse(`package main
 type Leaf struct { Count int }
 type Pair struct { Left, Right *Leaf; Value Leaf }
 func main() {
     var pair Pair
     println(pair.Left)
     println(pair.Right)
     println(pair.Value.Count)
 }
 `, ssaapi.WithLanguage(ssaconfig.GO))
	require.NoError(t, err)
	var got []string
	for _, call := range prog.Ref("println").GetUsers() {
		if operand := call.GetOperand(1); operand != nil {
			got = append(got, operand.String())
		}
	}
	require.Equal(t, []string{"nil", "nil", "0"}, got)
}

func TestGoZeroValueRecursivePointerFields(t *testing.T) {
	prog, err := ssaapi.Parse(`package main
 type A struct { B *B; N int }
 type B struct { C *C }
 type C struct { D *D }
 type D struct { A *A }
 func main() {
     var a A
     var b B
     var c C
     var d D
     println(a.B)
     println(b.C)
     println(c.D)
     println(d.A)
     println(a.N)
 }
 `, ssaapi.WithLanguage(ssaconfig.GO))
	require.NoError(t, err)
	var got []string
	for _, call := range prog.Ref("println").GetUsers() {
		if operand := call.GetOperand(1); operand != nil {
			got = append(got, operand.String())
		}
	}
	require.Equal(t, []string{"nil", "nil", "nil", "nil", "0"}, got)
}
