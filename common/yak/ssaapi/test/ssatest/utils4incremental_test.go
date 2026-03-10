package ssatest

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestCheckIncrementalProgram_Basic(t *testing.T) {
	step1Checks := 0
	step2Checks := 0
	step3Checks := 0

	CheckIncrementalProgram(t,
		IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  public String getValue() {
    return "base-v1";
  }
}`,
				"Main.java": `
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println("step2");
    System.out.println(a.getValue());
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, _ IncrementalCheckStage) {
				step1Checks++
				require.NotNil(t, overlay)
				res, err := overlay.SyntaxFlowWithError(`"base-v1" as $target`)
				require.NoError(t, err)
				require.NotNil(t, res)
				CompareResult(t, true, res, map[string][]string{
					"target": {"base-v1"},
				})
			},
		},
		IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  public String getValue() {
    return "diff-v2";
  }
}`,
				"Main.java": `
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, _ IncrementalCheckStage) {
				step2Checks++
				require.NotNil(t, overlay, "second step should create overlay")
				require.GreaterOrEqual(t, overlay.GetLayerCount(), 2)
				require.Greater(t, len(overlay.Layers), 0)

				res, err := overlay.SyntaxFlowWithError(`"diff-v2" as $target`)
				require.NoError(t, err)
				require.NotNil(t, res)
				CompareResult(t, true, res, map[string][]string{
					"target": {"diff-v2"},
				})
			},
		},
		IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  public String getValueV3() {
    return "diff-v3";
  }
}`,
				"Main.java": `
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println("step3");
    System.out.println(a.getValueV3());
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, _ IncrementalCheckStage) {
				step3Checks++
				require.NotNil(t, overlay, "third step should keep overlay")
				require.GreaterOrEqual(t, overlay.GetLayerCount(), 2)
				require.Greater(t, len(overlay.Layers), 0)

				res, err := overlay.SyntaxFlowWithError(`"diff-v3" as $target`)
				require.NoError(t, err)
				require.NotNil(t, res)
				CompareResult(t, true, res, map[string][]string{
					"target": {"diff-v3"},
				})
			},
		},
	)

	require.Equal(t, 2, step1Checks)
	require.Equal(t, 2, step2Checks)
	require.Equal(t, 2, step3Checks)
}
