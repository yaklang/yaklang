package ssadb

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// The pool key covers the kind, so values that render the same text restore
// with their own raw Go type instead of collapsing into one row.
func TestIrConstPoolDistinguishesKinds(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "const-kinds"

	type want struct {
		kind IrConstKind
		text string
		raw  any
	}
	cases := []want{
		{IrConstKindNil, "nil", nil},
		{IrConstKindString, "nil", "nil"},
		{IrConstKindInt64, "1", int64(1)},
		{IrConstKindString, "1", "1"},
		{IrConstKindBool, "true", true},
		{IrConstKindString, "true", "true"},
		{IrConstKindString, "", ""},
	}

	ids := make(map[string]int64, len(cases))
	for _, c := range cases {
		id := InternIrConst(prog, c.kind, c.text)
		require.NotZero(t, id, "kind=%d text=%q", c.kind, c.text)
		ids[fmt.Sprintf("%d/%s", c.kind, c.text)] = id
	}
	require.NotEqual(t, ids["0/nil"], ids["4/nil"], "nil and \"nil\" must not share a pool row")
	require.NotEqual(t, ids["2/1"], ids["4/1"], "int 1 and \"1\" must not share a pool row")
	require.NotEqual(t, ids["1/true"], ids["4/true"], "bool true and \"true\" must not share a pool row")

	require.NoError(t, GetIrConstPool(prog).Flush(db))
	for _, c := range cases {
		id := ids[fmt.Sprintf("%d/%s", c.kind, c.text)]
		value := GetIrConstValue(db, prog, id)
		require.NotNil(t, value, "kind=%d text=%q", c.kind, c.text)
		require.Equal(t, c.kind, value.Kind, "kind=%d text=%q", c.kind, c.text)
		require.Equal(t, c.raw, value.Raw(), "kind=%d text=%q", c.kind, c.text)
	}
}

// flushIrConstPools has to serve rows assembled outside the codec. It reads
// const_value instead of guessing a string kind: "nil" and "1" are ambiguous
// once the raw value is gone.
func TestFlushIrConstPoolsReadsKindFromExtra(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "const-fallback"

	rows := []*IrCode{
		{CodeID: 1, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "nil", ExtraInformation: `{"const_value":null}`},
		{CodeID: 2, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "nil", ExtraInformation: `{"const_value":"nil"}`},
		{CodeID: 3, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "1", ExtraInformation: `{"const_value":1}`},
		{CodeID: 4, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "1", ExtraInformation: `{"const_value":"1"}`},
		{CodeID: 5, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "true", ExtraInformation: `{"const_value":true}`},
		{CodeID: 6, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "", ExtraInformation: `{"const_value":""}`},
	}
	require.NoError(t, SaveIrCodeBatch(db, rows))

	expected := map[int64]any{
		1: nil,
		2: "nil",
		3: int64(1),
		4: "1",
		5: true,
		6: "",
	}
	for codeID, raw := range expected {
		var stored IrCode
		require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog, codeID).First(&stored).Error)
		require.NotZero(t, stored.ConstID, "code_id=%d must be pooled from const_value", codeID)
		value := GetIrConstValue(db, prog, stored.ConstID)
		require.NotNil(t, value, "code_id=%d", codeID)
		require.Equal(t, raw, value.Raw(), "code_id=%d", codeID)
	}

	// Without const_value the kind is unknown. The row must stay unpooled
	// instead of being stored as a string.
	unknown := &IrCode{CodeID: 7, ProgramName: prog, Opcode: irCodeConstOpcode,
		ConstType: irCodeConstTypeNormal, String: "unknown"}
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{unknown}))
	var stored IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog, 7).First(&stored).Error)
	require.Zero(t, stored.ConstID, "a row without const_value must not be guessed as a string")
}
