package javascript

import (
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
	"github.com/yaklang/yaklang/common/log"
)

var _ Visitor = (*walker)(nil)

type ASTWalkerResult struct {
	StringLiteral  []string
	Int64Literal   []int64
	Float64Literal []float64
	Identifies     []string
	BadSyntax      []string
}

// ASTWalk 对传入的JS代码进行AST遍历，返回遍历后的结果(包含字面量，标识符，语法错误)和错误
// Example:
// ```
// code = `function add(a, b) { return a + b; }`
// res = js.ASTWalk(code)~
// dump(res)
// ```
func BasicJavaScriptASTWalker(code string) (*ASTWalkerResult, error) {
	walker := NewJavaScriptWalker()
	walker.code = code
	results := &ASTWalkerResult{}
	walker.OnStringLiteral = func(i string) {
		results.StringLiteral = append(results.StringLiteral, i)
	}
	walker.OnInt64Literal = func(i int64) {
		results.Int64Literal = append(results.Int64Literal, i)
	}
	walker.OnFloat64Literal = func(i float64) {
		results.Float64Literal = append(results.Float64Literal, i)
	}
	walker.OnIdentifier = func(i string, n ast.Node) {
		results.Identifies = append(results.Identifies, i)
	}
	walker.OnSyntaxError = func(i string, lastNode ast.Node) {
		results.BadSyntax = append(results.BadSyntax, i)
	}
	astProgram, err := parser.ParseFile(nil, "", code, 0)
	if err != nil {
		return nil, err
	}

	Walk(walker, astProgram)
	return results, nil
}

type walker struct {
	code     string
	lastNode ast.Node
	handlers []func(n ast.Node)

	OnInt64Literal   func(i int64)
	OnFloat64Literal func(i float64)
	OnStringLiteral  func(i string)
	OnIdentifier     func(i string, lastNode ast.Node)
	OnSyntaxError    func(i string, lastNode ast.Node)
}

func (w *walker) init() {
	if w.OnStringLiteral == nil {
		w.OnStringLiteral = func(i string) {
		}
	}

	if w.OnInt64Literal == nil {
		w.OnInt64Literal = func(i int64) {
		}
	}

	if w.OnFloat64Literal == nil {
		w.OnFloat64Literal = func(i float64) {
		}
	}

	if w.OnIdentifier == nil {
		w.OnIdentifier = func(i string, N ast.Node) {
		}
	}

	if w.OnSyntaxError == nil {
		w.OnSyntaxError = func(i string, lastNode ast.Node) {
		}
	}
}

func NewJavaScriptWalker(handlers ...func(ast.Node)) *walker {
	w := &walker{handlers: handlers}
	w.init()
	return w
}

func (w *walker) Enter(n ast.Node) Visitor {
	defer func() {
		w.lastNode = n

		if err := recover(); err != nil {
			log.Warnf("javascript ast walk error: %s", err)
		}
	}()

	for _, handler := range w.handlers {
		handler(n)
	}

	switch ret := n.(type) {
	case *ast.BadStatement:
		if ret.From < ret.To && (len(w.code)-1) >= int(ret.From) {
			w.OnSyntaxError(w.code[ret.From:ret.To], ret)
		}
	case *ast.BadExpression:
		if ret.From < ret.To && (len(w.code)-1) >= int(ret.From) {
			if len(w.code) > int(ret.To) {
				w.OnSyntaxError(w.code[ret.From:ret.To], ret)
			} else {
				w.OnSyntaxError(w.code[ret.From:], ret)
			}
		}
	case *ast.StringLiteral:
		w.OnStringLiteral(ret.Value.String())
	case *ast.NumberLiteral:
		switch number := ret.Value.(type) {
		case float64:
			w.OnFloat64Literal(number)
		case int64:
			w.OnInt64Literal(number)
		default:
			log.Errorf("cannot supported: %v", ret)
		}
	case *ast.Identifier:
		if ret == nil {
			return w
		}
		w.OnIdentifier(ret.Name.String(), w.lastNode)
	}
	return w
}

func (w *walker) Exit(n ast.Node) {
}
