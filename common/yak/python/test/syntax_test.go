package test

import (
	"embed"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
)

//go:embed code
var codeFs embed.FS

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		_, err := python2ssa.Frontend(src)
		require.Nil(t, err, "parse AST FrontEnd error : %v", err)
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validateSource(t, tc.name, tc.code)
		})
	}
}
