package values

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

func TestNewExpressionMultidimensionalArrayString(t *testing.T) {
	funcCtx := &class_context.ClassContext{}
	stringClass := types.NewJavaClass("java.lang.String")
	array2d := types.NewJavaArrayType(types.NewJavaArrayType(stringClass))
	length2 := NewCustomValue(func(funcCtx *class_context.ClassContext) string { return "2" }, func() types.JavaType {
		return types.NewJavaPrimer(types.JavaInteger)
	})
	length4 := NewCustomValue(func(funcCtx *class_context.ClassContext) string { return "4" }, func() types.JavaType {
		return types.NewJavaPrimer(types.JavaInteger)
	})

	exp := NewNewArrayExpression(array2d, length2, length4)
	got := exp.String(funcCtx)
	require.Equal(t, "new String[2][4]", got)
}
