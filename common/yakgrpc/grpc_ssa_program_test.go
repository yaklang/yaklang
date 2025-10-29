package yakgrpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func query(t *testing.T, filter *ypb.SSAProgramFilter, local ypb.YakClient, name string) *ypb.SSAProgram {
	res, err := local.QuerySSAPrograms(context.Background(), &ypb.QuerySSAProgramRequest{
		Filter: filter,
	})
	require.NoError(t, err)
	for _, prog := range res.Data {
		if prog.Name == name {
			return prog
		}
	}
	return nil
}

func TestGRPCMUSTPASS_SyntaxFlow_SSAPrograms_Query(t *testing.T) {
	name := uuid.NewString()
	desc := `
	this is simple yaklang code example 
	`

	prog, err := ssaapi.Parse(`print("a")`,
		ssaapi.WithProgramName(name),
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramDescription(desc),
	)
	_ = prog
	defer func() {

		ssadb.DeleteProgram(ssadb.GetDB(), name)
	}()
	require.NoError(t, err)

	local, err := NewLocalClient(true)
	require.NoError(t, err)

	queryByFilter := func(t *testing.T, filter *ypb.SSAProgramFilter) *ypb.SSAProgram {
		return query(t, filter, local, name)
	}

	t.Run("query all", func(t *testing.T) {
		prog := queryByFilter(t, nil)
		require.NotNil(t, prog)
		require.Equal(t, prog.Name, name)
		require.Equal(t, prog.Language, string(ssaconfig.Yak))
		require.Equal(t, prog.Description, desc)
	})
	t.Run("query with filter name", func(t *testing.T) {
		require.NotNil(t, queryByFilter(t, &ypb.SSAProgramFilter{
			ProgramNames: []string{name},
		}))
	})
	t.Run("query with filter description", func(t *testing.T) {
		require.NotNil(t, queryByFilter(t, &ypb.SSAProgramFilter{
			Keyword: "simple",
		}))
	})
	t.Run("query with Language", func(t *testing.T) {
		require.NotNil(t, queryByFilter(t, &ypb.SSAProgramFilter{
			Languages: []string{string(ssaconfig.Yak)},
		}))
	})
	t.Run("query risk by filter", func(t *testing.T) {
		require.Nil(t, queryByFilter(t, &ypb.SSAProgramFilter{
			Languages: []string{string(ssaconfig.JAVA)},
		}))
	})

	t.Run("query risk", func(t *testing.T) {
		res, err := prog.SyntaxFlowWithError(`
		print(* as $a)
		alert $a for {
			level: 'high',
		}
		`)
		require.NoError(t, err)
		resultId, err := res.Save(schema.SFResultKindDebug)
		_ = resultId
		require.NoError(t, err)

		ssaProg := queryByFilter(t, nil)
		require.NotNil(t, ssaProg)
		require.Equal(t, ssaProg.HighRiskNumber, int64(1))
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_Program_Delete_WithKeyword(t *testing.T) {
	name := uuid.NewString()
	desc := `
	this is simple yaklang code example 
	`
	local, err := NewLocalClient(true)
	require.NoError(t, err)

	_, err = ssaapi.Parse(`print("a")`,
		ssaapi.WithProgramName(name),
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramDescription(desc),
	)
	defer func() {

		ssadb.DeleteProgram(ssadb.GetDB(), name)
	}()
	require.NoError(t, err)

	have := func(targetName string) bool {
		return query(t, nil, local, targetName) != nil
	}

	// have this program
	require.True(t, have(name))

	// delete program
	_, err = local.DeleteSSAPrograms(context.Background(), &ypb.DeleteSSAProgramRequest{
		Filter: &ypb.SSAProgramFilter{
			ProgramNames: []string{name},
		},
	})
	require.NoError(t, err)

	// no this program
	require.False(t, have(name))
}

func TestGRPCMUSTPASS_SyntaxFlow_Program_Update(t *testing.T) {

	name := uuid.NewString()
	desc := `
	this is simple yaklang code example 
	`
	local, err := NewLocalClient(true)
	require.NoError(t, err)

	_, err = ssaapi.Parse(`print("a")`,
		ssaapi.WithProgramName(name),
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramDescription(desc),
	)
	defer func() {

		ssadb.DeleteProgram(ssadb.GetDB(), name)
	}()
	require.NoError(t, err)

	get := func() *ypb.SSAProgram {
		return query(t, nil, local, name)
	}
	prog := get()
	require.NotNil(t, prog)
	require.Equal(t, prog.Description, desc)

	newDesc := "new desc"
	_, err = local.UpdateSSAProgram(context.Background(), &ypb.UpdateSSAProgramRequest{
		ProgramInput: &ypb.SSAProgramInput{
			Name:        name,
			Description: newDesc,
		},
	})
	require.NoError(t, err)

	prog = get()
	require.NotNil(t, prog)
	require.Equal(t, prog.Description, newDesc)
}
