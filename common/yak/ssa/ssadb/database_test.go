package ssadb_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestSqliteID(t *testing.T) {
	db := consts.GetGormProjectDatabase().Debug()
	projectName := uuid.NewString()
	id, _ := ssadb.RequireIrCode(db, projectName)
	id2, _ := ssadb.RequireIrCode(db, projectName)
	defer ssadb.DeleteProgram(db, projectName)

	require.Equal(t, id+1, id2)
}
