package reducer

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestReducer(t *testing.T) {

	reduceLimit := 5
	reduceHandler := func(data []string) string {
		return "reduced"
	}

	reducer := NewReducer(reduceLimit, reduceHandler)

	for i := 0; i < 10; i++ {
		reducer.Push("data" + utils.RandStringBytes(10))
	}

	require.Equal(t, reduceLimit, len(reducer.data))
	require.Equal(t, "reduced", reducer.data[0].Value())
	require.Equal(t, 10, len(reducer.allData))
	fmt.Println(reducer.Dump())

}
