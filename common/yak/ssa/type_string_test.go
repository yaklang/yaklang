package ssa

import (
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestTypeStringConcurrentRecursiveGraphs(t *testing.T) {
	object := NewStructType()
	object.FieldTypes = []Type{object}
	function := NewFunctionType("", nil, nil, false)
	function.ReturnType = NewChanType(function)
	for _, test := range []struct {
		typ  Type
		want string
	}{
		{object, "struct {...}"},
		{function, "() -> chan ..."},
	} {
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 32; j++ {
					require.Equal(t, test.want, test.typ.String())
				}
			}()
		}
		wg.Wait()
	}
	require.Empty(t, object.Name)
	require.Empty(t, function.Name)
}
