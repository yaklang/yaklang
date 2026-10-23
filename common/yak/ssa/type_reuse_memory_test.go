package ssa

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestBasicTypeSparseMutableState(t *testing.T) {
	t.Logf("BasicType: %d bytes", unsafe.Sizeof(BasicType{}))
	require.LessOrEqual(t, unsafe.Sizeof(BasicType{}), uintptr(96), "basic types must not carry unused method locks, maps and initialization state")
	a, b := NewBasicType(StringTypeKind, "string"), NewBasicType(StringTypeKind, "string")
	require.Same(t, a.description, b.description, "immutable type descriptions must be reused")
	a.SetId(20)
	a.AddMethod("local", nil)
	a.AddFullTypeName("pkg.Local")
	require.EqualValues(t, -1, b.GetId())
	require.Empty(t, b.GetMethod())
	require.Empty(t, b.GetFullTypeNames())
}

func TestTypeStringsSharedInMemoryAndDatabase(t *testing.T) {
	for _, mode := range []ProgramCacheKind{ProgramCacheMemory, ProgramCacheDBWrite} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName("type-reuse-"+fmt.Sprint(mode)))
			require.NoError(t, err)
			prog := NewProgram(cfg, mode, Application, filesys.NewVirtualFs(), "", 1)
			if mode == ProgramCacheDBWrite {
				defer ssadb.DeleteProgram(ssadb.GetDB(), prog.Name)
				defer closeUnsavedFlushFixture(t, prog)()
			}
			b := prog.GetAndCreateFunctionBuilder("", "strings")
			left, right := b.EmitUndefined("left"), b.EmitUndefined("right")
			a := NewBasicType(StringTypeKind, strings.Clone("reuse.TypeName"))
			c := NewBasicType(StringTypeKind, strings.Clone("reuse.TypeName"))
			a.AddFullTypeName(strings.Clone("reuse.QualifiedName"))
			c.AddFullTypeName(strings.Clone("reuse.QualifiedName"))
			left.SetType(a)
			right.SetType(c)
			require.True(t, unsafe.StringData(a.String()) == unsafe.StringData(c.String()), "equal type names must share string backing storage")
			require.True(t, unsafe.StringData(a.GetFullTypeNames()[0]) == unsafe.StringData(c.GetFullTypeNames()[0]), "equal qualified names must share string backing storage")
			a.AddFullTypeName("reuse.OnlyLeft")
			require.Equal(t, []string{"reuse.QualifiedName"}, c.GetFullTypeNames())
			require.NotEqual(t, a.GetId(), c.GetId(), "mutable types retain independent identities")
			b.Finish()
		})
	}
}

func TestTypeStoreExistingIDReservesCounter(t *testing.T) {
	p := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, p)()
	existing := NewBasicType(StringTypeKind, "existing")
	existing.SetId(100001)
	p.Cache.rememberType(existing)
	next := CreateNumberType()
	p.Cache.rememberType(next)
	require.Greater(t, next.GetId(), existing.GetId(), "remembered IDs must not collide with a subsequent new type")
}
