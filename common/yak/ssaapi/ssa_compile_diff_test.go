package ssaapi_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCompileDiffProgramAndSaveToDB(t *testing.T) {
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Base";
  public String getValue() {
    return "Value from A";
  }
}`,
				"Main.java": `
public class Main{
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
  }
}`,
				"Utils.java": `
public class Utils {
  public static void helper() {
    System.out.println("Helper from Utils");
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, _ ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				res, err := overlay.SyntaxFlowWithError(`"Value from Base" as $target`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, res, map[string][]string{
					"target": {"Value from Base"},
				})
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Diff";
  public String getValue() {
    return "Value from Modified A";
  }
}`,
				"B.java": `
public class B {
  public static void process() {
    System.out.println("Process from B");
  }
}`,
				"Utils.java": "",
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageCompile {
					return
				}
				require.NotNil(t, overlay)

				layerNames := overlay.GetLayerProgramNames()
				require.GreaterOrEqual(t, len(layerNames), 2)
				baseName := layerNames[0]
				diffName := layerNames[len(layerNames)-1]
				require.NotEqual(t, baseName, diffName)

				diffProgram, err := ssaapi.FromDatabase(diffName)
				require.NoError(t, err)
				require.NotNil(t, diffProgram)
				require.NotNil(t, diffProgram.Program)
				require.Equal(t, baseName, diffProgram.Program.BaseProgramName)

				fileHashMap := diffProgram.Program.FileHashMap
				require.NotNil(t, fileHashMap)
				require.Equal(t, 0, fileHashMap["A.java"])
				require.Equal(t, 1, fileHashMap["B.java"])
				require.Equal(t, -1, fileHashMap["Utils.java"])
				require.NotContains(t, fileHashMap, "Main.java")

				require.NotEmpty(t, diffProgram.Ref("A"))
				require.NotEmpty(t, diffProgram.Ref("B"))
				require.Empty(t, diffProgram.Ref("Utils"))

				irProg, err := ssadb.GetProgram(diffName, ssadb.Application)
				require.NoError(t, err)
				require.NotNil(t, irProg)
				require.True(t, irProg.IsOverlay)
			},
		},
	)
}

func TestIncrementalCompile_Twice(t *testing.T) {
	t.Run("test incremental compile twice", func(t *testing.T) {
		ssatest.CheckIncrementalProgram(t,
			ssatest.IncrementalStep{
				Files: map[string]string{
					"A.java": `
public class A {
  static string valueStr = "Value from Base";
  public String getValue() {
    return "Value from A";
  }
}`,
					"Main.java": `
public class Main{
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
  }
}`,
					"Utils.java": `
public class Utils {
  public static void helper() {
    System.out.println("Helper from Utils");
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
					"Utils.java": "",
				},
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageCompile {
						return
					}
					require.NotNil(t, overlay)
					layerNames := overlay.GetLayerProgramNames()
					require.GreaterOrEqual(t, len(layerNames), 2)
					diffName := layerNames[len(layerNames)-1]
					irProg, err := ssadb.GetProgram(diffName, ssadb.Application)
					require.NoError(t, err)
					require.NotNil(t, irProg)
					require.Equal(t, "0", irProg.FileHashMap["A.java"])
					require.Equal(t, "1", irProg.FileHashMap["B.java"])
					require.Equal(t, "-1", irProg.FileHashMap["Utils.java"])
					require.NotContains(t, irProg.FileHashMap, "Main.java")
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
					"C.java": `
public class C {
  public static void compute() {
    System.out.println("Compute from C");
  }
}`,
					"B.java": "",
				},
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageCompile {
						return
					}
					require.NotNil(t, overlay)
					require.GreaterOrEqual(t, overlay.GetLayerCount(), 3)
					layerNames := overlay.GetLayerProgramNames()
					require.GreaterOrEqual(t, len(layerNames), 3)
					diffName := layerNames[len(layerNames)-1]
					irProg, err := ssadb.GetProgram(diffName, ssadb.Application)
					require.NoError(t, err)
					require.NotNil(t, irProg)
					require.Equal(t, "0", irProg.FileHashMap["A.java"])
					require.Equal(t, "1", irProg.FileHashMap["C.java"])
					require.Equal(t, "-1", irProg.FileHashMap["B.java"])
					require.NotContains(t, irProg.FileHashMap, "Main.java")

					cRes, err := overlay.SyntaxFlowWithError(`"Compute from C" as $target`)
					require.NoError(t, err)
					ssatest.CompareResult(t, true, cRes, map[string][]string{
						"target": {"Compute from C"},
					})
					bRes, err := overlay.SyntaxFlowWithError(`B<sourceCode> as $target`)
					require.NoError(t, err)
					require.Empty(t, bRes.GetValues("target"))
				},
			},
		)
	})

	t.Run("test incremental compile add then delete file", func(t *testing.T) {
		ssatest.CheckIncrementalProgram(t,
			ssatest.IncrementalStep{
				Files: map[string]string{
					"A.java": `
public class A {
  public String getValue() {
    return "Value from A";
  }
}`,
					"Main.java": `
public class Main{
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
  }
}`,
				},
			},
			ssatest.IncrementalStep{
				Files: map[string]string{
					"Temp.java": `
public class Temp {
  public static void process() {
    System.out.println("Process from Temp");
  }
}`,
				},
			},
			ssatest.IncrementalStep{
				Files: map[string]string{
					"A.java": `
public class A {
  // Modified in diff2 to ensure diffFS is not empty
  public String getValue() {
    return "Value from A";
  }
}`,
					"Temp.java": "",
				},
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageCompile {
						return
					}
					require.NotNil(t, overlay)
					layerNames := overlay.GetLayerProgramNames()
					require.GreaterOrEqual(t, len(layerNames), 3)
					diffName := layerNames[len(layerNames)-1]
					irProg, err := ssadb.GetProgram(diffName, ssadb.Application)
					require.NoError(t, err)
					require.NotNil(t, irProg)
					require.Equal(t, "-1", irProg.FileHashMap["Temp.java"])

					tempRes, err := overlay.SyntaxFlowWithError(`Temp as $res`)
					require.NoError(t, err)
					require.Empty(t, tempRes.GetValues("res"))
					aRes, err := overlay.SyntaxFlowWithError(`A as $res`)
					require.NoError(t, err)
					ssatest.CompareResult(t, true, aRes, map[string][]string{
						"res": {"A"},
					})
				},
			},
		)
	})
}

func TestIsOverlayFieldInDatabase(t *testing.T) {
	t.Run("test base program IsOverlay false", func(t *testing.T) {
		var baseProgramName string
		var diffProgramName string

		ssatest.CheckIncrementalProgramWithOptions(t,
			[]ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.JAVA)},
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
				Options: []ssaconfig.Option{
					ssaapi.WithEnableIncrementalCompile(false),
				},
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageCompile {
						return
					}
					require.NotNil(t, overlay)
					layerNames := overlay.GetLayerProgramNames()
					require.NotEmpty(t, layerNames)
					baseProgramName = layerNames[0]

					baseIrProg, err := ssadb.GetProgram(baseProgramName, ssadb.Application)
					require.NoError(t, err)
					require.NotNil(t, baseIrProg)
					require.False(t, baseIrProg.IsOverlay)
				},
			},
			ssatest.IncrementalStep{
				Files: map[string]string{
					"A.java": `
public class A {
  static string valueStr = "Value from Diff";
  public String getValue() {
    return "Value from Modified A";
  }
}`,
				},
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageCompile {
						return
					}
					require.NotNil(t, overlay)
					layerNames := overlay.GetLayerProgramNames()
					require.GreaterOrEqual(t, len(layerNames), 2)
					diffProgramName = layerNames[len(layerNames)-1]

					baseIrProg, err := ssadb.GetProgram(baseProgramName, ssadb.Application)
					require.NoError(t, err)
					require.NotNil(t, baseIrProg)
					require.False(t, baseIrProg.IsOverlay)

					diffIrProg, err := ssadb.GetProgram(diffProgramName, ssadb.Application)
					require.NoError(t, err)
					require.NotNil(t, diffIrProg)
					require.True(t, diffIrProg.IsOverlay)
				},
			},
		)
	})

	t.Run("test base program IsOverlay true with incremental compile", func(t *testing.T) {
		ssatest.CheckIncrementalProgramWithOptions(t,
			[]ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.JAVA)},
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
				Options: []ssaconfig.Option{
					ssaapi.WithEnableIncrementalCompile(true),
				},
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageCompile {
						return
					}
					require.NotNil(t, overlay)
					layerNames := overlay.GetLayerProgramNames()
					require.NotEmpty(t, layerNames)
					baseIrProg, err := ssadb.GetProgram(layerNames[0], ssadb.Application)
					require.NoError(t, err)
					require.NotNil(t, baseIrProg)
					require.True(t, baseIrProg.IsOverlay)
				},
			},
		)
	})
}
