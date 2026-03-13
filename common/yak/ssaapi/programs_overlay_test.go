package ssaapi_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestProgramOverLay_MultiLayerAndReload(t *testing.T) {
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
				"Utils.java": `
public class Utils {
  public static void helper() {
    System.out.println("helper");
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
    System.out.println("process");
  }
}`,
				"Utils.java": "",
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
				"C.java": `
public class C {
  public static void compute() {
    System.out.println("compute");
  }
}`,
				"B.java": "",
			},
			Check: func(overlay *ssaapi.ProgramOverLay, _ ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				require.GreaterOrEqual(t, overlay.GetLayerCount(), 2)

				layerNames := overlay.GetLayerProgramNames()
				require.GreaterOrEqual(t, len(layerNames), 2)
				require.NotEmpty(t, layerNames[len(layerNames)-1])

				aggFS := overlay.GetAggregatedFileSystem()
				require.NotNil(t, aggFS)
				fileSet := make(map[string]bool)
				filesys.Recursive(".", filesys.WithFileSystem(aggFS), filesys.WithFileStat(func(p string, info fs.FileInfo) error {
					if info.IsDir() {
						return nil
					}
					if len(p) > 0 && p[0] == '/' {
						p = p[1:]
					}
					fileSet[p] = true
					return nil
				}))

				require.True(t, hasOverlayFileBySuffix(fileSet, "A.java"))
				require.False(t, hasOverlayFileBySuffix(fileSet, "Utils.java"))

				require.NotEmpty(t, overlay.Ref("A"))
				require.Empty(t, overlay.Ref("Utils"))
			},
		},
	)
}

func hasOverlayFileBySuffix(fileSet map[string]bool, name string) bool {
	for path := range fileSet {
		if path == name || strings.HasSuffix(path, "/"+name) {
			return true
		}
	}
	return false
}

func TestProgramOverLay_SyntaxFlowAcrossLayers(t *testing.T) {
	rule := `println(, * as $arg); $arg #-> as $data`
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
				"Main.java": `
public class Main {
  public static void main(String[] args) {
    System.out.println("base");
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, _ ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				baseRes, err := overlay.SyntaxFlowWithError(rule)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, baseRes, map[string][]string{
					"data": {"base"},
				})
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"Main.java": `
public class Main {
  public static void main(String[] args) {
    System.out.println("diff");
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, _ ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				require.GreaterOrEqual(t, len(overlay.Layers), 2)
				overlayRes, err := overlay.SyntaxFlowWithError(rule)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, overlayRes, map[string][]string{
					"data": {"diff"},
				})
			},
		},
	)
}

func TestOverlay_Easy(t *testing.T) {
	var baseValue *ssaapi.Value
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
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				baseRes, err := overlay.SyntaxFlowWithError(`valueStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, baseRes, map[string][]string{
					"res": {"Value from Base"},
				})

				if stage != ssatest.IncrementalCheckStageDB || baseValue != nil {
					return
				}
				baseValues := overlay.Ref("valueStr")
				require.NotEmpty(t, baseValues)
				baseValue = baseValues[0]
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
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage == ssatest.IncrementalCheckStageDB {
					return
				}
				require.NotNil(t, overlay)
				overlayRes, err := overlay.SyntaxFlowWithError(`valueStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, overlayRes, map[string][]string{
					"res": {"Value from Extend"},
				})

				require.NotNil(t, baseValue)
				relocated := overlay.Relocate(baseValue)
				require.NotNil(t, relocated)

				layerNames := overlay.GetLayerProgramNames()
				require.GreaterOrEqual(t, len(layerNames), 2)
				require.Equal(t, layerNames[len(layerNames)-1], relocated.GetProgramName())
			},
		},
	)
}

func TestOverlay_StringLiteralUsesVisibleLayer(t *testing.T) {
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Base";
}`,
				"Config.java": `
public class Config {
  static string configValue = "Config from Base";
}`,
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Diff";
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, _ ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)

				diffRes, err := overlay.SyntaxFlowWithError(`"Value from Diff" as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, diffRes, map[string][]string{
					"res": {"Value from Diff"},
				})

				baseRes, err := overlay.SyntaxFlowWithError(`"Value from Base" as $res`)
				require.NoError(t, err)
				require.Empty(t, baseRes.GetValues("res"))

				configRes, err := overlay.SyntaxFlowWithError(`"Config from Base" as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, configRes, map[string][]string{
					"res": {"Config from Base"},
				})
			},
		},
	)
}

func TestOverlay_CrossLayer_Flow(t *testing.T) {
	rule := `println(, * as $arg); $arg #-> as $data`
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
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
  }
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, _ ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				baseRes, err := overlay.SyntaxFlowWithError(rule)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, baseRes, map[string][]string{
					"data": {"Value from A"},
				})
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
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
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage == ssatest.IncrementalCheckStageDB {
					return
				}
				require.NotNil(t, overlay)
				overlayRes, err := overlay.SyntaxFlowWithError(rule)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, overlayRes, map[string][]string{
					"data": {"Value from Extended A"},
				})
				for _, value := range overlayRes.GetValues("data") {
					require.NotContains(t, value.String(), "Value from A")
				}
			},
		},
	)
}

func TestOverlay_FileSystem(t *testing.T) {
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
			Check: func(overlay *ssaapi.ProgramOverLay, _ ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				aggFS := overlay.GetAggregatedFileSystem()
				require.NotNil(t, aggFS)
				fileSet := make(map[string]bool)
				filesys.Recursive(".", filesys.WithFileSystem(aggFS), filesys.WithFileStat(func(p string, info fs.FileInfo) error {
					if info.IsDir() {
						return nil
					}
					if len(p) > 0 && p[0] == '/' {
						p = p[1:]
					}
					fileSet[p] = true
					return nil
				}))
				require.True(t, hasOverlayFileBySuffix(fileSet, "A.java"))
				require.True(t, hasOverlayFileBySuffix(fileSet, "Main.java"))
				require.True(t, hasOverlayFileBySuffix(fileSet, "NewFile.java"))
				require.False(t, hasOverlayFileBySuffix(fileSet, "Utils.java"))

				newFileRes, err := overlay.SyntaxFlowWithError(`NewFile as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, newFileRes, map[string][]string{
					"res": {"NewFile"},
				})
				mainRes, err := overlay.SyntaxFlowWithError(`Main as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, mainRes, map[string][]string{
					"res": {"Main"},
				})
				utilsRes, err := overlay.SyntaxFlowWithError(`Utils as $res`)
				require.NoError(t, err)
				require.Empty(t, utilsRes.GetValues("res"))
			},
		},
	)
}

func TestOverlay_FileSystem_FromDataBase(t *testing.T) {
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
				if stage != ssatest.IncrementalCheckStageDB {
					return
				}
				require.NotNil(t, overlay)
				aggFS := overlay.GetAggregatedFileSystem()
				require.NotNil(t, aggFS)
				fileSet := make(map[string]bool)
				filesys.Recursive(".", filesys.WithFileSystem(aggFS), filesys.WithFileStat(func(p string, info fs.FileInfo) error {
					if info.IsDir() {
						return nil
					}
					if len(p) > 0 && p[0] == '/' {
						p = p[1:]
					}
					fileSet[p] = true
					return nil
				}))
				require.True(t, hasOverlayFileBySuffix(fileSet, "A.java"))
				require.True(t, hasOverlayFileBySuffix(fileSet, "Main.java"))
				require.True(t, hasOverlayFileBySuffix(fileSet, "NewFile.java"))
				require.False(t, hasOverlayFileBySuffix(fileSet, "Utils.java"))
			},
		},
	)
}

func TestOverlay_SyntaxFlowRule(t *testing.T) {
	ruleName1 := "golang-" + uuid.NewString() + ".sf"
	ruleContent1 := `
http?{<fullTypeName>?{have: "net/http"}} as $http
$http.ListenAndServe as $mid
`
	ruleName2 := "golang-" + uuid.NewString() + ".sf"
	ruleContent2 := `
exec?{<fullTypeName>?{have: 'os/exec'}} as $entry
$entry.Command(* #-> as $sink)
r?{<fullTypeName>?{have: 'net/http'}}.URL.Query().Get(* #-> as $input)
$sink & $input as $high
`
	rule1, err := sfdb.CreateRuleByContent(ruleName1, ruleContent1, false)
	require.NoError(t, err)
	rule2, err := sfdb.CreateRuleByContent(ruleName2, ruleContent2, false)
	require.NoError(t, err)
	defer func() {
		sfdb.DeleteRuleByRuleName(ruleName1)
		sfdb.DeleteRuleByRuleName(ruleName2)
	}()

	ssatest.CheckIncrementalProgramWithOptions(t, []ssaconfig.Option{
		ssaconfig.WithProjectLanguage(ssaconfig.GO),
	}, ssatest.IncrementalStep{
		Files: map[string]string{
			"main.go": `package main

import (
	"net/http"
)

func main() {
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}`,
			"safe.go": `package main

import "os/exec"

func safe(input string) string {
	cmd := exec.Command("echo", input)
	out, _ := cmd.CombinedOutput()
	return string(out)
}`,
		},
	}, ssatest.IncrementalStep{
		Files: map[string]string{
			"main.go": `package main

import (
	"net/http"
)

func main() {
	http.HandleFunc("/unsafe", unsafeHandler)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}`,
			"unsafe.go": `package main

import (
	"net/http"
	"os/exec"
)

func unsafeHandler(w http.ResponseWriter, r *http.Request) {
	cmdParam := r.URL.Query().Get("cmd")
	cmd := exec.Command("sh", "-c", "echo "+cmdParam)
	_, _ = cmd.CombinedOutput()
}`,
			"safe.go": "",
		},
		Check: func(overlay *ssaapi.ProgramOverLay, _ ssatest.IncrementalCheckStage) {
			require.NotNil(t, overlay)

			ruleRes1, err := overlay.SyntaxFlowRule(rule1)
			require.NoError(t, err)
			ssatest.CompareResult(t, true, ruleRes1, map[string][]string{
				"mid": {"ListenAndServe"},
			})
			ruleRes2, err := overlay.SyntaxFlowRule(rule2)
			require.NoError(t, err)
			ssatest.CompareResult(t, true, ruleRes2, map[string][]string{
				"high": {"cmd"},
			})

			contentRes1, err := overlay.SyntaxFlowWithError(ruleContent1)
			require.NoError(t, err)
			ssatest.CompareResult(t, true, contentRes1, map[string][]string{
				"mid": {"ListenAndServe"},
			})
			contentRes2, err := overlay.SyntaxFlowWithError(ruleContent2)
			require.NoError(t, err)
			ssatest.CompareResult(t, true, contentRes2, map[string][]string{
				"high": {"cmd"},
			})
		},
	})
}
