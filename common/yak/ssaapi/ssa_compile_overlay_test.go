package ssaapi_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestOverlaySaveAndLoadFromDatabase(t *testing.T) {
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
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
  }
}`,
				"Utils.java": `
public class Utils {
  public static void helper() {
    System.out.println("Helper from Base");
  }
}`,
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Extend";
  public String getValue() {
    return "Value from Extended A";
  }
}`,
				"Main.java": `
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
  }
}`,
				"NewFile.java": `
public class NewFile {
  public static void newMethod() {
    System.out.println("New method from Extend");
  }
}`,
				"Utils.java": "",
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				require.GreaterOrEqual(t, overlay.GetLayerCount(), 2)

				layerNames := overlay.GetLayerProgramNames()
				require.GreaterOrEqual(t, len(layerNames), 2)
				latestName := layerNames[len(layerNames)-1]
				require.NotEmpty(t, latestName)

				latestIR, err := ssadb.GetProgram(latestName, ssadb.Application)
				require.NoError(t, err)
				require.NotNil(t, latestIR)
				require.True(t, latestIR.IsOverlay)

				agg := overlay.GetAggregatedFileSystem()
				require.NotNil(t, agg)
				fileSet := map[string]bool{}
				filesys.Recursive(".", filesys.WithFileSystem(agg), filesys.WithFileStat(func(p string, info fs.FileInfo) error {
					if !info.IsDir() {
						if len(p) > 0 && p[0] == '/' {
							p = p[1:]
						}
						fileSet[p] = true
					}
					return nil
				}))

				require.True(t, hasFileBySuffix(fileSet, "A.java"))
				require.True(t, hasFileBySuffix(fileSet, "Main.java"))
				require.True(t, hasFileBySuffix(fileSet, "NewFile.java"))
				require.False(t, hasFileBySuffix(fileSet, "Utils.java"))

				require.NotEmpty(t, overlay.Ref("A"))
				require.Empty(t, overlay.Ref("Utils"))

				if stage == ssatest.IncrementalCheckStageDB {
					require.GreaterOrEqual(t, len(layerNames), 2)
				}
			},
		},
	)
}

func TestOverlayWithMultipleLayers(t *testing.T) {
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  public String getValue() {
    return "base";
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
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  public String getValue() {
    return "diff-1";
  }
}`,
				"B.java": `
public class B {
  public static void process() {
    System.out.println("Process from B");
  }
}`,
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  public String getValue() {
    return "diff-2";
  }
}`,
				"B.java": `
public class B {
  public static void process() {
    System.out.println("Process from B v2");
  }
}`,
				"C.java": `
public class C {
  public static void compute() {
    System.out.println("Compute from C");
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				require.GreaterOrEqual(t, overlay.GetLayerCount(), 2)
				require.NotEmpty(t, overlay.Ref("A"))

				if stage == ssatest.IncrementalCheckStageCompile {
					require.GreaterOrEqual(t, len(overlay.GetLayerProgramNames()), 2)
				}
			},
		},
	)
}

func TestOverlayWithTwiceIncrementalCompile(t *testing.T) {
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
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
  }
}`,
				"Utils.java": `
public class Utils {
  public static void helper() {
    System.out.println("Helper from Base");
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
				"Main.java": `
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
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
				"Main.java": `
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
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
				require.NotEmpty(t, overlay.Ref("A"))
				require.NotEmpty(t, overlay.Ref("C"))
				require.Empty(t, overlay.Ref("B"))
				require.Empty(t, overlay.Ref("Utils"))

				agg := overlay.GetAggregatedFileSystem()
				require.NotNil(t, agg)
				fileSet := map[string]bool{}
				filesys.Recursive(".", filesys.WithFileSystem(agg), filesys.WithFileStat(func(p string, info fs.FileInfo) error {
					if info.IsDir() {
						return nil
					}
					if len(p) > 0 && p[0] == '/' {
						p = p[1:]
					}
					fileSet[p] = true
					return nil
				}))
				require.True(t, hasFileBySuffix(fileSet, "A.java"))
				require.True(t, hasFileBySuffix(fileSet, "Main.java"))
				require.True(t, hasFileBySuffix(fileSet, "C.java"))
				require.False(t, hasFileBySuffix(fileSet, "B.java"))
				require.False(t, hasFileBySuffix(fileSet, "Utils.java"))
			},
		},
	)
}

func hasFileBySuffix(fileSet map[string]bool, name string) bool {
	for path := range fileSet {
		if path == name || strings.HasSuffix(path, "/"+name) {
			return true
		}
	}
	return false
}
