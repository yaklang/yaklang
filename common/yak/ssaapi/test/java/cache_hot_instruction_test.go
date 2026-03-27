package java

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func buildAggressiveJavaCacheFS(helperCount int) *filesys.VirtualFS {
	vf := filesys.NewVirtualFs()

	mainCode := "package demo;\n\npublic class Main {\n    public static void println(String v) {}\n    public void run() {\n"
	for i := 0; i < helperCount; i++ {
		mainCode += fmt.Sprintf("        println(new Helper%02d().value());\n", i)
	}
	mainCode += `
        Holder left = new Holder("left");
        Holder right = new Holder("right");
        Holder selected;
        if (left.value().length() > 0) {
            selected = left;
        } else {
            selected = right;
        }
        println(selected.value());
`
	mainCode += "    }\n}\n"
	vf.AddFile("demo/Main.java", mainCode)

	vf.AddFile("demo/Holder.java", `package demo;

public class Holder {
    private final String value;

    public Holder(String value) {
        this.value = value;
    }

    public String value() {
        return value;
    }
}
`)

	for i := 0; i < helperCount; i++ {
		helperName := fmt.Sprintf("Helper%02d", i)
		helperValue := fmt.Sprintf("helper-%02d", i)
		vf.AddFile(
			fmt.Sprintf("demo/%s.java", helperName),
			fmt.Sprintf(`package demo;

public class %s {
    public %s() {}

    public String value() {
        return "%s";
    }
}
`, helperName, helperName, helperValue),
		)
	}

	return vf
}

func TestJavaCompileWithAggressiveCacheKeepsHotInstructions(t *testing.T) {
	vf := buildAggressiveJavaCacheFS(48)

	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		result, err := programs.SyntaxFlowWithError(`println(* #-> * as $param)`)
		require.NoError(t, err)

		got := make(map[string]struct{})
		for _, value := range result.GetValues("param") {
			if constant := value.GetConstValue(); constant != nil {
				got[fmt.Sprint(constant)] = struct{}{}
			}
		}
		for i := 0; i < 48; i++ {
			_, ok := got[fmt.Sprintf("helper-%02d", i)]
			require.Truef(t, ok, "missing propagated helper constant helper-%02d", i)
		}
		_, ok := got["left"]
		require.True(t, ok, "missing propagated phi-member constant left")

		ctorResult, err := programs.SyntaxFlowWithError(`
Helper00() as $ctor0;
Helper47() as $ctor47;
Holder(* #-> * as $holder_ctor_arg) as $holderCtor;
`)
		require.NoError(t, err)
		require.NotEmpty(t, ctorResult.GetValues("ctor0"))
		require.NotEmpty(t, ctorResult.GetValues("ctor47"))
		require.NotEmpty(t, ctorResult.GetValues("holderCtor"))
		require.NotEmpty(t, ctorResult.GetValues("holder_ctor_arg"))
		return nil
	},
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaconfig.WithCompileIrCacheTTL(2*time.Millisecond),
		ssaconfig.WithCompileIrCacheMax(8),
	)
}
