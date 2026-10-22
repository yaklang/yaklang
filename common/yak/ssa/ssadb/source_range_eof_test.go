package ssadb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// Python whole-file/suite ranges are half-open and can end exactly at EOF.
// Persist and reload that boundary instead of dropping it as "out of range".
func TestPersistedSourceRangeAtEOF(t *testing.T) {
	for _, source := range []string{"", "value", "value\n", "变量 = 1\n"} {
		t.Run(source, func(t *testing.T) {
			program := uuid.NewString()
			t.Cleanup(func() { DeleteProgram(GetDB(), program) })
			editor := memedit.NewMemEditor(source)
			editor.SetProgramName(program)
			editor.SetFileName("sample.py")
			require.NoError(t, MarshalFile(editor).Save(GetDB()))
			full := editor.GetFullRange()
			rows := []interface {
				GetStartAndEndPositions() (*memedit.MemEditor, *memedit.Position, *memedit.Position, error)
			}{
				&IrOffset{FileHash: editor.GetIrSourceHash(), StartOffset: 0, EndOffset: int64(full.GetEndOffset())},
				&IrOffset{FileHash: editor.GetIrSourceHash(), StartOffset: int64(full.GetEndOffset()), EndOffset: int64(full.GetEndOffset())},
				&IrCode{SourceCodeHash: editor.GetIrSourceHash(), SourceCodeStartOffset: 0, SourceCodeEndOffset: int64(full.GetEndOffset())},
			}
			for _, row := range rows {
				gotEditor, start, end, err := row.GetStartAndEndPositions()
				require.NoError(t, err, "EOF is a valid exclusive source boundary")
				require.NotNil(t, start)
				require.Equal(t, full.GetEnd().GetLine(), end.GetLine())
				require.Equal(t, full.GetEnd().GetColumn(), end.GetColumn())
				require.Equal(t, editor.GetIrSourceHash(), gotEditor.GetIrSourceHash())
			}
			invalid := &IrOffset{FileHash: editor.GetIrSourceHash(), EndOffset: int64(full.GetEndOffset() + 1)}
			_, _, _, err := invalid.GetStartAndEndPositions()
			require.Error(t, err, "genuinely corrupt offsets must remain errors, not get clamped")
		})
	}
}
