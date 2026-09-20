package ssa

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFullTypeNamesConcurrentUpdates(t *testing.T) {
	for _, typ := range []Type{NewBlueprint("C"), NewBasicType(StringTypeKind, "string"), NewAnyFunctionType(), NewObjectType(), NewInterfaceType("I", "pkg")} {
		t.Run(fmt.Sprintf("%T", typ), func(t *testing.T) {
			var wg sync.WaitGroup
			for worker := 0; worker < 8; worker++ {
				wg.Add(1)
				go func(worker int) {
					defer wg.Done()
					for i := 0; i < 16; i++ {
						typ.AddFullTypeName(fmt.Sprintf("pkg%d.Type%d", worker, i))
						_ = typ.GetFullTypeNames()
						_ = typ.String()
					}
				}(worker)
			}
			wg.Wait()
			before := typ.GetFullTypeNames()
			require.Len(t, before, 128)
			typ.SetFullTypeNames([]string{"replacement"})
			require.Len(t, before, 128, "a published snapshot must survive replacement")
			require.Equal(t, []string{"replacement"}, typ.GetFullTypeNames())
		})
	}
}
