package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetProgramFreshnessLightQuery verifies the lightweight freshness query:
// it returns updated_at and compile_generation when the row exists, and
// found=false (no error) when the program row does not exist (review A11).
func TestGetProgramFreshnessLightQuery(t *testing.T) {
	oldDB := GetDB()
	db := openRepairTestDB(t)
	require.NoError(t, db.AutoMigrate(&IrProgram{}).Error)
	SetDB(db)
	t.Cleanup(func() { SetDB(oldDB) })

	prog := &IrProgram{ProgramName: "freshness-prog", ProgramKind: Application}
	require.NoError(t, db.Create(prog).Error)

	updatedAt, generation, found, err := GetProgramFreshness("freshness-prog", Application)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, updatedAt.IsZero(), "created row must have an updated_at")
	require.Zero(t, generation, "fresh rows start at generation 0")

	_, _, found, err = GetProgramFreshness("missing-prog", Application)
	require.NoError(t, err)
	require.False(t, found, "missing program must report not-found without error")
}
