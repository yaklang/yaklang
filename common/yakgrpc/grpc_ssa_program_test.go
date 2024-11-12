package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestSsaProgramsQuery(t *testing.T) {
	name := uuid.NewString()
	version := "test-version"
	db := consts.GetGormProfileDatabase()
	yakit.CreateSsaProgram(db, &schema.SSAProgram{
		Name:          name,
		Description:   "description",
		Language:      "java",
		EngineVersion: version,
	})
	ssadb.CreateProgram(name, "application", version)
	defer func() {
		yakit.DeleteSsaProgramWithName(db, name)
	}()
	queryCheck := func(t *testing.T, programs []*ypb.SsaProgram) {
		var flag bool
		for _, program := range programs {
			if program.GetName() == name {
				flag = true
			}
		}
		require.True(t, flag)
	}
	t.Run("query all", func(t *testing.T) {
		client, err := NewLocalClient(true)
		require.NoError(t, err)
		programs, err := client.QuerySsaPrograms(context.Background(), &ypb.QuerySsaProgramRequest{
			IsAll: true,
		})
		require.NoError(t, err)
		queryCheck(t, programs.Programs)
	})
	t.Run("query with filter name", func(t *testing.T) {
		client, err := NewLocalClient(true)
		require.NoError(t, err)
		programs, err := client.QuerySsaPrograms(context.Background(), &ypb.QuerySsaProgramRequest{
			Filter: &ypb.SsaProgramFilter{
				ProgramName: name,
			},
		})
		require.NoError(t, err)
		queryCheck(t, programs.Programs)
	})
	t.Run("query with filter description", func(t *testing.T) {
		client, err := NewLocalClient(true)
		require.NoError(t, err)
		programs, err := client.QuerySsaPrograms(context.Background(), &ypb.QuerySsaProgramRequest{Filter: &ypb.SsaProgramFilter{
			Keyword: "crip",
		}})
		require.NoError(t, err)
		queryCheck(t, programs.Programs)
	})
	t.Run("query with Language", func(t *testing.T) {
		client, err := NewLocalClient(true)
		require.NoError(t, err)
		programs, err := client.QuerySsaPrograms(context.Background(), &ypb.QuerySsaProgramRequest{Filter: &ypb.SsaProgramFilter{
			Language: "java",
		}})
		require.NoError(t, err)
		programs2, err2 := client.QuerySsaPrograms(context.Background(), &ypb.QuerySsaProgramRequest{Filter: &ypb.SsaProgramFilter{
			Language: "php",
		}})
		require.NoError(t, err2)
		require.True(t, programs2.Total == 0)
		queryCheck(t, programs.Programs)
	})
}

func TestDeleteProgramDeleteAll(t *testing.T) {
	name := uuid.NewString()
	version := "test-version"
	db := consts.GetGormProfileDatabase()
	yakit.CreateSsaProgram(db, &schema.SSAProgram{
		Name:          name,
		Description:   "description",
		Language:      "java",
		EngineVersion: version,
	})
	ssadb.CreateProgram(name, "application", version)
	err := yakit.DeleteSsaProgram(db, &ypb.DeleteSsaProgramRequest{
		IsAll: true,
	})
	require.NoError(t, err)
	program, err := ssadb.GetProgram(name, "")
	require.Error(t, err)
	require.True(t, program == nil)
}

func TestDeleteProgramWithKeyword(t *testing.T) {
	name := uuid.NewString()
	version := "test-version"
	db := consts.GetGormProfileDatabase()
	yakit.CreateSsaProgram(db, &schema.SSAProgram{
		Name:          name,
		Description:   "description",
		Language:      "java",
		EngineVersion: version,
	})
	ssadb.CreateProgram(name, "application", version)
	err := yakit.DeleteSsaProgram(db, &ypb.DeleteSsaProgramRequest{
		Filter: &ypb.SsaProgramFilter{
			Keyword: "desc",
		},
	})
	require.NoError(t, err)
	program, err := ssadb.GetProgram(name, "")
	require.Error(t, err)
	require.True(t, program == nil)
}
