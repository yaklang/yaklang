package yakit

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestSSA_Program_SSAProgramExistCheck(t *testing.T) {
	tokenName := utils.RandStringBytes(10)
	db := consts.GetGormProfileDatabase()

	db.Model(&schema.SSAProgram{}).Save(&schema.SSAProgram{
		Name:        tokenName,
		Description: "test",
		DBPath:      utils.RandStringBytes(10),
	})
	_, data, err := QuerySSAProgram(db, &ypb.QuerySSAProgramRequest{Filter: &ypb.SSAProgramFilter{
		ProgramNames: []string{tokenName},
	}})
	require.NoError(t, err)
	require.Lenf(t, data, 1, "data: %v", data)

	SSAProgramExistClear(db)

	_, data, err = QuerySSAProgram(db, &ypb.QuerySSAProgramRequest{Filter: &ypb.SSAProgramFilter{
		ProgramNames: []string{tokenName},
	}})
	require.NoError(t, err)
	require.Lenf(t, data, 0, "data: %v", data)

}
