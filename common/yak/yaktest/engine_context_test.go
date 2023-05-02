package yaktest

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
)

func testScriptWithTimeout(duration time.Duration, t *testing.T, code string, expectErrorInfo string) {
	ctx, _ := context.WithTimeout(context.Background(), duration)
	scriptEngine := yak.NewScriptEngine(1)
	err := scriptEngine.ExecuteWithContext(ctx, code)
	if expectErrorInfo == "" {
		if err != nil {
			t.Fatal(err)
		}
	} else if err == nil || !utils.MatchAllOfGlob(err.Error(), expectErrorInfo) {
		t.Fatal(utils.Errorf("expect %v error, but got: %v", expectErrorInfo, err))
	}

}
func TestScriptEngine(t *testing.T) {
	testScriptWithTimeout(400*time.Millisecond, t, `time.sleep(0.5)`, "context deadline exceeded") // 测试超时
	testScriptWithTimeout(700*time.Millisecond, t, `time.sleep(0.5)`, "")                          // 测试不超时
}
func TestImport(t *testing.T) {
	var fp, err = os.CreateTemp(os.TempDir(), "import-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func(path string) {
		if err := fp.Close(); err != nil {
			t.Fatal(err)
		}
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(fp.Name())
	if _, err := fp.WriteString(`
a = 1
println("start import")
time.sleep(0.5)
println("end import")
`); err != nil {
		t.Fatal(err)
	}
	var fileName string
	if runtime.GOOS == "windows" {
		fileName = strings.Replace(fp.Name(), "\\", "\\\\", -1)
	} else {
		fileName = fp.Name()
	}
	testScriptWithTimeout(700*time.Millisecond, t, fmt.Sprintf(`a = import("%s","a")~`, fileName), "")
	testScriptWithTimeout(400*time.Millisecond, t, fmt.Sprintf(`a = import("%s","a")~`, fileName), "*native func `import` call error*failed: context deadline exceeded*")
}
func TestDyn(t *testing.T) {
	var fp, err = os.CreateTemp(os.TempDir(), "import-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func(path string) {
		if err := fp.Close(); err != nil {
			t.Fatal(err)
		}
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(fp.Name())
	if _, err := fp.WriteString(`
a = 1
println("start import")
time.sleep(0.5)
println("end import")
`); err != nil {
		t.Fatal(err)
	}
	var fileName string
	if runtime.GOOS == "windows" {
		fileName = strings.Replace(fp.Name(), "\\", "\\\\", -1)
	} else {
		fileName = fp.Name()
	}
	testScriptWithTimeout(700*time.Millisecond, t, fmt.Sprintf(`a = dyn.Import("%s","a")~`, fileName), "")
	testScriptWithTimeout(400*time.Millisecond, t, fmt.Sprintf(`a = dyn.Import("%s","a")~`, fileName), "*YakVM Panic: native func `dyn.Import` call error: load file * failed: context deadline exceeded*")
}
func TestEval(t *testing.T) {
	testScriptWithTimeout(700*time.Millisecond, t, `eval("time.sleep(0.5)")`, "")
	testScriptWithTimeout(400*time.Millisecond, t, `eval("time.sleep(0.5)")`, "*YakVM Panic: context deadline exceeded*")
}
