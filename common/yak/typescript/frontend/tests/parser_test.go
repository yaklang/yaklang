package tests

import (
	"embed"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/parser"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
	"gotest.tools/v3/assert"
	"io/fs"
	"testing"
)

//go:embed testdata/*
var embeddedFiles embed.FS

func printAllChildren(node *ast.Node, depth int) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "----"
	}

	fmt.Printf("%s %s [%d, %d)\n", indent, node.Kind.String(), node.Pos(), node.End())

	children := []*ast.Node{}
	node.VisitEachChild(ast.NewNodeVisitor(func(node *ast.Node) *ast.Node {
		children = append(children, node)
		return node
	}, nil, ast.NodeVisitorHooks{}))
	for _, child := range children {
		printAllChildren(child, depth+1)
	}
}

// helper 生成唯一ID
var nextID int

func genID() int {
	nextID++
	return nextID
}

type dotNode struct {
	ID   int
	Kind string
	Pos  int
	End  int
}

func TestParseTypeScript(t *testing.T) {
	t.Parallel()
	tests := make([]struct {
		name         string
		code         string
		ignoreErrors bool
	}, 0)
	// 获取所有 .ts 文件的内容
	tsFiles, err := fs.Glob(embeddedFiles, "testdata/*.ts")
	if err != nil {
		t.Fatalf("failed to find .ts files: %v", err)
	}

	// 遍历所有匹配到的文件
	for _, file := range tsFiles {
		fileContent, err := embeddedFiles.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", file, err)
		}
		tests = append(tests, struct {
			name         string
			code         string
			ignoreErrors bool
		}{
			name:         file,
			code:         string(fileContent),
			ignoreErrors: false,
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sf := parser.ParseSourceFile(test.name, "", test.code, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
			if !test.ignoreErrors {
				assert.Equal(t, len(sf.Diagnostics()), 0)
			}
		})
	}
}

func TestParseTypeScriptX(t *testing.T) {
	t.Parallel()
	tests := make([]struct {
		name         string
		code         string
		ignoreErrors bool
	}, 0)
	// 获取所有 .ts 文件的内容
	tsFiles, err := fs.Glob(embeddedFiles, "testdata/*.tsx")
	if err != nil {
		t.Fatalf("failed to find .ts files: %v", err)
	}

	// 遍历所有匹配到的文件
	for _, file := range tsFiles {
		fileContent, err := embeddedFiles.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", file, err)
		}
		tests = append(tests, struct {
			name         string
			code         string
			ignoreErrors bool
		}{
			name:         file,
			code:         string(fileContent),
			ignoreErrors: false,
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sf := parser.ParseSourceFile(test.name, "", test.code, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
			if !test.ignoreErrors {
				assert.Equal(t, len(sf.Diagnostics()), 0)
			}
		})
	}
}

func TestParseTypeScriptDeclFile(t *testing.T) {
	t.Parallel()
	tests := make([]struct {
		name         string
		code         string
		ignoreErrors bool
	}, 0)
	// 获取所有 .ts 文件的内容
	tsFiles, err := fs.Glob(embeddedFiles, "testdata/*.d.ts")
	if err != nil {
		t.Fatalf("failed to find .ts files: %v", err)
	}

	// 遍历所有匹配到的文件
	for _, file := range tsFiles {
		fileContent, err := embeddedFiles.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", file, err)
		}
		tests = append(tests, struct {
			name         string
			code         string
			ignoreErrors bool
		}{
			name:         file,
			code:         string(fileContent),
			ignoreErrors: false,
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sf := parser.ParseSourceFile(test.name, "", test.code, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
			if !test.ignoreErrors {
				assert.Equal(t, len(sf.Diagnostics()), 0)
			}
		})
	}
}

func TestParseJavaScript(t *testing.T) {
	t.Parallel()
	tests := make([]struct {
		name         string
		code         string
		ignoreErrors bool
	}, 0)
	// 获取所有 .ts 文件的内容
	tsFiles, err := fs.Glob(embeddedFiles, "testdata/*.js")
	if err != nil {
		t.Fatalf("failed to find .ts files: %v", err)
	}

	// 遍历所有匹配到的文件
	for _, file := range tsFiles {
		fileContent, err := embeddedFiles.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", file, err)
		}
		tests = append(tests, struct {
			name         string
			code         string
			ignoreErrors bool
		}{
			name:         file,
			code:         string(fileContent),
			ignoreErrors: false,
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sf := parser.ParseSourceFile(test.name, "", test.code, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
			if !test.ignoreErrors {
				assert.Equal(t, len(sf.Diagnostics()), 0)
			}
		})
	}
}
