package java2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Merging a name that is already present must keep the list unchanged and in
// order. The implementation now relies on AddFullTypeName's deduplication
// instead of snapshotting the whole list for every candidate name.
func TestMergeFullTypeNameForTypeIsIdempotent(t *testing.T) {
	builder := &singleFileBuilder{}
	typ := ssa.NewBasicType(ssa.StringTypeKind, "String")
	typ.SetFullTypeNames([]string{"java.lang.String", "java.io.Serializable"})

	merged := builder.MergeFullTypeNameForType(
		[]string{"java.lang.String", "java.lang.Object", "java.lang.Object"}, typ)
	require.Equal(t, []string{"java.lang.String", "java.io.Serializable", "java.lang.Object"},
		merged.GetFullTypeNames())

	again := builder.MergeFullTypeNameForType([]string{"java.lang.Object"}, merged)
	require.Equal(t, merged.GetFullTypeNames(), again.GetFullTypeNames())
}
