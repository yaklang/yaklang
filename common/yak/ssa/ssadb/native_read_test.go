package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func setupNativeReadTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrCode{}, &IrType{}).Error)
	return db
}

func TestNativeGetIrTypeItemById_Equivalent(t *testing.T) {
	db := setupNativeReadTestDB(t)
	prog := "native-type-prog"
	item := &IrType{
		TypeId:           42,
		Kind:             3,
		ProgramName:      prog,
		String:           "java/lang/String",
		ExtraInformation: `{"name":"x"}`,
	}
	require.NoError(t, db.Create(item).Error)

	native := nativeGetIrTypeItemById(db, prog, 42)
	require.NotNil(t, native)
	require.Equal(t, item.TypeId, native.TypeId)
	require.Equal(t, item.Kind, native.Kind)
	require.Equal(t, item.ProgramName, native.ProgramName)
	require.Equal(t, item.String, native.String)
	require.Equal(t, item.ExtraInformation, native.ExtraInformation)

	// nonexistent id
	require.Nil(t, nativeGetIrTypeItemById(db, prog, 999))
	// negative id
	require.Nil(t, nativeGetIrTypeItemById(db, prog, -1))
}

func TestNativeGetIrCodeItemById_Equivalent(t *testing.T) {
	db := setupNativeReadTestDB(t)
	prog := "native-code-prog"
	item := &IrCode{
		CodeID:           7,
		ProgramName:      prog,
		Opcode:           3,
		OpcodeName:       "Call",
		Name:             "f",
		String:           "f()",
		Users:            Int64Slice{1, 2},
		FormalArgs:       Int64Slice{10},
		ObjectMembers:    Int64Map{{10, 20}},
		Variable:         StringSlice{"x", "y"},
		IsFunction:       true,
		CurrentBlock:     5,
		TypeID:           9,
		ExtraInformation: `{"k":"v"}`,
	}
	require.NoError(t, db.Create(item).Error)

	native := nativeGetIrCodeItemById(db, prog, 7)
	require.NotNil(t, native)
	require.Equal(t, item.CodeID, native.CodeID)
	require.Equal(t, item.ProgramName, native.ProgramName)
	require.Equal(t, item.Opcode, native.Opcode)
	require.Equal(t, item.OpcodeName, native.OpcodeName)
	require.Equal(t, item.Name, native.Name)
	require.Equal(t, item.String, native.String)
	require.Equal(t, []int64(item.Users), []int64(native.Users))
	require.Equal(t, []int64(item.FormalArgs), []int64(native.FormalArgs))
	require.Equal(t, len(item.ObjectMembers), len(native.ObjectMembers))
	require.Equal(t, item.ObjectMembers[0].key, native.ObjectMembers[0].key)
	require.Equal(t, item.ObjectMembers[0].value, native.ObjectMembers[0].value)
	require.Equal(t, []string(item.Variable), []string(native.Variable))
	require.Equal(t, item.IsFunction, native.IsFunction)
	require.Equal(t, item.CurrentBlock, native.CurrentBlock)
	require.Equal(t, item.TypeID, native.TypeID)
	require.Equal(t, item.ExtraInformation, native.ExtraInformation)

	// nonexistent
	require.Nil(t, nativeGetIrCodeItemById(db, prog, 999))
}
