package test

import (
	"embed"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

//go:embed code
var codeFs embed.FS

var pythonTestAntlrCache = func() *ssa.AntlrCache {
	builder, ok := python2ssa.CreateBuilder().(*python2ssa.SSABuilder)
	if !ok {
		panic("python2ssa.CreateBuilder did not return *python2ssa.SSABuilder")
	}
	return builder.GetAntlrCache()
}()

func pythonFixtureParseBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv("YAK_PYTHON_FIXTURE_PARSE_BUDGET_SEC"))
	if raw == "" {
		return 30 * time.Second
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		start := time.Now()
		_, err := python2ssa.FrontendWithCache(src, pythonTestAntlrCache)
		elapsed := time.Since(start)
		require.Nil(t, err, "parse AST FrontEnd error : %v", err)
		if budget := pythonFixtureParseBudget(); budget > 0 && elapsed > budget {
			t.Fatalf("parse AST exceeded budget for %s: elapsed=%s budget=%s", filename, elapsed, budget)
		}
	})
}

func TestAllSyntaxForPython_G4(t *testing.T) {
	entry, err := codeFs.ReadDir("code")
	if err != nil {
		t.Fatalf("no embed syntax files found: %v", err)
	}
	for _, f := range entry {
		if f.IsDir() {
			continue
		}
		codePath := path.Join("code", f.Name())
		if !strings.HasSuffix(codePath, ".py") {
			continue
		}
		raw, err := codeFs.ReadFile(codePath)
		if err != nil {
			t.Fatalf("cannot found syntax fs: %v", codePath)
		}
		validateSource(t, codePath, string(raw))
	}
}

func TestBasicPythonSyntax(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "simple assignment",
			code: `x = 1
`,
		},
		{
			name: "function definition",
			code: `def hello():
    pass
`,
		},
		{
			name: "class definition",
			code: `class MyClass:
    pass
`,
		},
		{
			name: "if statement",
			code: `if True:
    pass
`,
		},
		{
			name: "for loop",
			code: `for i in range(10):
    print(i)
`,
		},
		{
			name: "walrus expression",
			code: `if username := config.get("username"):
    print(username)
`,
		},
		{
			name: "positional only slash parameters",
			code: `def callback(values: list[L], /, *_args: T2.args, **_kwargs: T2.kwargs) -> list[L]:
    return values
`,
		},
		{
			name: "for target star unpacking",
			code: `for head, tail, *rest in dependencies:
    print(head)
`,
		},
		{
			name: "parenthesized with items",
			code: `with (
    patch("a") as first,
    patch("b") as second,
):
    print(first)
`,
		},
		{
			name: "decimal underscore literal",
			code: `limit = 10_000
`,
		},
		{
			name: "match case statement",
			code: `match filter_method:
    case "width":
        print(filter_method)
    case "min" | "max":
        print(filter_method)
`,
		},
		{
			name: "match case guard and tuple pattern",
			code: `match name.partition("."):
    case filename, ".", "py" if filename.isidentifier():
        print(filename)
    case _:
        print(name)
`,
		},
		{
			name: "match case star pattern",
			code: `match data:
    case head, *rest:
        print(head)
    case _:
        print(data)
`,
		},
		{
			name: "type alias statement",
			code: `type ConfType = _dict[str, Any]
`,
		},
		{
			name: "function type parameters",
			code: `def is_unspecified[T](value: T | Sentinel) -> TypeGuard[Sentinel]:
    return value
`,
		},
		{
			name: "class type parameters",
			code: `class LocalProxy[T](WerkzeugLocalProxy):
    pass
`,
		},
		{
			name: "float underscore literal",
			code: `value = 0.420_001
`,
		},
		{
			name: "print as attribute name",
			code: `self.assertTrue(perm.print == 1)
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validateSource(t, tc.name, tc.code)
		})
	}
}
