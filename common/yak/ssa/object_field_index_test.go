package ssa

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestObjectWithFields(count int) *ObjectType {
	typ := NewObjectType()
	for i := 0; i < count; i++ {
		typ.AddField(NewConst(i), CreateNumberType())
	}
	return typ
}

// A field that was never added must be answered as absent. AddField keeps keymap
// in sync with Keys, so a complete keymap proves absence; the lookup must not
// fall back to scanning Keys for every miss, which is quadratic while a large
// initializer list is built field by field.
func TestObjectTypeMissingFieldLookupDoesNotScanKeys(t *testing.T) {
	const fields = 20000
	typ := newTestObjectWithFields(fields)

	start := time.Now()
	for i := 0; i < fields; i++ {
		require.Nil(t, typ.GetField(NewConst(fields+i)))
	}
	elapsed := time.Since(start)

	// Quadratic behavior needed hours here; a linear answer is microseconds.
	require.Less(t, elapsed, 5*time.Second, "absent-field lookup must not rescan every key")
}

func TestObjectTypeGetFieldFindsAddedFields(t *testing.T) {
	typ := NewObjectType()
	numberType := CreateNumberType()
	typ.AddField(NewConst("alpha"), numberType)

	require.Equal(t, numberType, typ.GetField(NewConst("alpha")))
	require.Nil(t, typ.GetField(NewConst("missing")))
}

// Call error-drop builds a struct type by assigning Keys/FieldTypes slices
// directly, then rebuilds the index before looking up the retained fields.
func TestObjectTypeGetFieldResolvesDirectlyAssignedKeys(t *testing.T) {
	source := NewObjectType()
	source.AddField(NewConst("first"), CreateNumberType())
	source.AddField(NewConst("second"), CreateStringType())

	truncated := NewStructType()
	truncated.Keys = append([]Value(nil), source.Keys...)
	truncated.FieldTypes = append([]Type(nil), source.FieldTypes...)
	require.Empty(t, truncated.keymap, "direct assignment must not populate keymap")
	truncated.rebuildKeyIndex()

	require.Equal(t, source.FieldTypes[1], truncated.GetField(NewConst("second")))
}

func TestObjectTypeAddFieldReplacesDuplicateKeyInPlace(t *testing.T) {
	typ := NewObjectType()
	typ.AddField(NewConst("key"), CreateNumberType())
	typ.AddField(NewConst("key"), CreateStringType())

	require.Len(t, typ.Keys, 1)
	require.Equal(t, StringTypeKind, typ.GetField(NewConst("key")).GetTypeKind())
}
