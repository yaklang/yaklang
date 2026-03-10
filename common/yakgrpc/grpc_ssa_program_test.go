package yakgrpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	yakgrpc "github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func queryProgramByName(t *testing.T, filter *ypb.SSAProgramFilter, local ypb.YakClient, name string) *ypb.SSAProgram {
	res, err := local.QuerySSAPrograms(context.Background(), &ypb.QuerySSAProgramRequest{Filter: filter})
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
	defer ssadb.DeleteProgram(ssadb.GetDB(), name)
	require.NoError(t, err)
	require.NotNil(t, prog)

	local, err := yakgrpc.NewLocalClient(true)
	require.NoError(t, err)

	queryByFilter := func(t *testing.T, filter *ypb.SSAProgramFilter) *ypb.SSAProgram {
		return queryProgramByName(t, filter, local, name)
	}

	t.Run("query all", func(t *testing.T) {
		program := queryByFilter(t, nil)
		require.NotNil(t, program)
		require.Equal(t, name, program.Name)
		require.Equal(t, string(ssaconfig.Yak), program.Language)
		require.Equal(t, desc, program.Description)
	})

	t.Run("query with filter name", func(t *testing.T) {
		require.NotNil(t, queryByFilter(t, &ypb.SSAProgramFilter{ProgramNames: []string{name}}))
	})

	t.Run("query with filter description", func(t *testing.T) {
		require.NotNil(t, queryByFilter(t, &ypb.SSAProgramFilter{Keyword: "simple"}))
	})

	t.Run("query with language", func(t *testing.T) {
		require.NotNil(t, queryByFilter(t, &ypb.SSAProgramFilter{Languages: []string{string(ssaconfig.Yak)}}))
	})

	t.Run("query risk", func(t *testing.T) {
		res, err := prog.SyntaxFlowWithError(`
		print(* as $a)
		alert $a for {
			level: 'high',
		}
		`)
		require.NoError(t, err)
		_, err = res.Save(schema.SFResultKindDebug)
		require.NoError(t, err)

		ssaProg := queryByFilter(t, nil)
		require.NotNil(t, ssaProg)
		require.Equal(t, int64(1), ssaProg.HighRiskNumber)
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_Program_Delete_WithKeyword(t *testing.T) {
	name := uuid.NewString()
	local, err := yakgrpc.NewLocalClient(true)
	require.NoError(t, err)

	_, err = ssaapi.Parse(`print("a")`,
		ssaapi.WithProgramName(name),
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramDescription("simple yaklang code example"),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), name)
	require.NoError(t, err)

	have := func(targetName string) bool {
		return queryProgramByName(t, nil, local, targetName) != nil
	}
	require.True(t, have(name))

	_, err = local.DeleteSSAPrograms(context.Background(), &ypb.DeleteSSAProgramRequest{
		Filter: &ypb.SSAProgramFilter{ProgramNames: []string{name}},
	})
	require.NoError(t, err)
	require.False(t, have(name))
}

func TestGRPCMUSTPASS_SyntaxFlow_Program_Update(t *testing.T) {
	name := uuid.NewString()
	local, err := yakgrpc.NewLocalClient(true)
	require.NoError(t, err)

	_, err = ssaapi.Parse(`print("a")`,
		ssaapi.WithProgramName(name),
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramDescription("simple yaklang code example"),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), name)
	require.NoError(t, err)

	get := func() *ypb.SSAProgram {
		return queryProgramByName(t, nil, local, name)
	}

	program := get()
	require.NotNil(t, program)
	require.Contains(t, program.Description, "simple")

	_, err = local.UpdateSSAProgram(context.Background(), &ypb.UpdateSSAProgramRequest{
		ProgramInput: &ypb.SSAProgramInput{
			Name:        name,
			Description: "new desc",
		},
	})
	require.NoError(t, err)

	program = get()
	require.NotNil(t, program)
	require.Equal(t, "new desc", program.Description)
}

func TestGRPCMUSTPASS_SyntaxFlow_SSAPrograms_QueryIncrementalCompileInfo(t *testing.T) {
	var (
		baseProgramName string
		diff1Program    string
		diff2Program    string
		normalProgram   string
	)

	ssatest.CheckIncrementalProgramWithOptions(t,
		[]ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithContext(context.Background())},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Base";
  public String getValue() {
    return "Value from A";
  }
}`,
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Diff1";
  public String getValue() {
    return "Value from Modified A in Diff1";
  }
}`,
				"B.java": `
public class B {
  public static void process() {
    System.out.println("Process from B");
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageCompile || overlay == nil {
					return
				}
				names := overlay.GetLayerProgramNames()
				require.GreaterOrEqual(t, len(names), 2)
				baseProgramName = names[0]
				diff1Program = names[len(names)-1]
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Diff2";
  public String getValue() {
    return "Value from Modified A in Diff2";
  }
}`,
				"B.java": "",
				"C.java": `
public class C {
  public static void compute() {
    System.out.println("Compute from C");
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageCompile || overlay == nil {
					return
				}
				names := overlay.GetLayerProgramNames()
				require.GreaterOrEqual(t, len(names), 2)
				diff2Program = names[len(names)-1]
			},
		},
	)

	require.NotEmpty(t, baseProgramName)
	require.NotEmpty(t, diff1Program)
	require.NotEmpty(t, diff2Program)

	ssatest.CheckIncrementalProgramWithOptions(t,
		[]ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithContext(context.Background())},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"Normal.java": `
public class Normal {
  public static void main(String[] args) {
    System.out.println("Normal compilation");
  }
}`,
			},
			Options: []ssaconfig.Option{
				ssaapi.WithEnableIncrementalCompile(false),
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageCompile || overlay == nil {
					return
				}
				names := overlay.GetLayerProgramNames()
				require.NotEmpty(t, names)
				normalProgram = names[0]
			},
		},
	)
	require.NotEmpty(t, normalProgram)

	local, err := yakgrpc.NewLocalClient(true)
	require.NoError(t, err)

	programNames := []string{baseProgramName, diff1Program, diff2Program, normalProgram}
	res, err := local.QuerySSAPrograms(context.Background(), &ypb.QuerySSAProgramRequest{
		Filter: &ypb.SSAProgramFilter{ProgramNames: programNames},
	})
	require.NoError(t, err)
	require.Len(t, res.Data, len(programNames))

	programMap := make(map[string]*ypb.SSAProgram)
	for _, prog := range res.Data {
		programMap[prog.Name] = prog
	}

	for _, name := range []string{baseProgramName, diff1Program, diff2Program} {
		prog, ok := programMap[name]
		require.True(t, ok)
		require.True(t, prog.IsIncrementalCompile)
		require.Equal(t, baseProgramName, prog.IncrementalGroupId)
		require.Equal(t, diff2Program, prog.HeadProgramName)
	}

	normal, ok := programMap[normalProgram]
	require.True(t, ok)
	require.False(t, normal.IsIncrementalCompile)
	require.Equal(t, normalProgram, normal.IncrementalGroupId)
	require.Equal(t, normalProgram, normal.HeadProgramName)
}
