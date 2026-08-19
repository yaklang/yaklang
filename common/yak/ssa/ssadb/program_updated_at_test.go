package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetProgramUpdatedAtLightQuery verifies the lightweight freshness query:
// it returns the program's updated_at when the row exists, and found=false
// (no error) when the program row does not exist (review A11).
func TestGetProgramUpdatedAtLightQuery(t *testing.T) {
	oldDB := GetDB()
	db := openRepairTestDB(t)
	require.NoError(t, db.AutoMigrate(&IrProgram{}).Error)
	SetDB(db)
	t.Cleanup(func() { SetDB(oldDB) })

	prog := &IrProgram{ProgramName: "freshness-prog", ProgramKind: Application}
	require.NoError(t, db.Create(prog).Error)

	updatedAt, found, err := GetProgramUpdatedAt("freshness-prog", Application)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, updatedAt.IsZero(), "created row must have an updated_at")

	_, found, err = GetProgramUpdatedAt("missing-prog", Application)
	require.NoError(t, err)
	require.False(t, found, "missing program must report not-found without error")
}
