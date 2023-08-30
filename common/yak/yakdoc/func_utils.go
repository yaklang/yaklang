package yakdoc

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
)

var (
	InterfaceToAnyRegep, _ = regexp.Compile(`interface\s*\{\}`)
)

// rename native type
func shrinkTypeVerboseName(i string) string {
	if InterfaceToAnyRegep.MatchString(i) {
		return InterfaceToAnyRegep.ReplaceAllString(i, "any")
	}

	return i
}

// Get the name and path of a func
func funcPathAndName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// Get the name of a func (with package path)
func funcName(f interface{}) string {
	splitFuncName := strings.Split(funcPathAndName(f), ".")
	return splitFuncName[len(splitFuncName)-1]
}

// Get description and declaration of a func
func funcDescriptionAndDeclaration(f interface{}, overideName string, debug ...string) (document, declaration, autoCompletion string) {
	fv := reflect.ValueOf(f)
	if fv.Kind() != reflect.Func {
		return "", "", ""
	}
	pc := fv.Pointer()
	if pc == 0 {
		return "", "", ""
	}
	function := runtime.FuncForPC(pc)
	if function == nil {
		return "", "", ""
	}

	fileName, _ := function.FileLine(0)
	funcName := funcName(f)
	fset := token.NewFileSet()

	// Parse src
	parsedAst, err := parser.ParseFile(fset, fileName, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
		return "", "", ""
	}

	pkg := &ast.Package{
		Name:  "Any",
		Files: make(map[string]*ast.File),
	}
	pkg.Files[fileName] = parsedAst

	importPath, _ := filepath.Abs("/")
	myDoc := doc.New(pkg, importPath, doc.AllDecls)
	for _, theFunc := range myDoc.Funcs {
		if theFunc.Name == funcName {

			decl := theFunc.Decl
			start := fset.Position(decl.Pos())
			end := fset.Position(decl.End())
			b, err := ioutil.ReadFile(fileName)
			if err != nil {
				panic(err)
			}
			declaration = string(b[start.Offset:end.Offset])
			// 去除func和函数名
			funcIndex := strings.Index(declaration, "func ")
			declaration = declaration[funcIndex+5:]
			leftparenIndex := strings.Index(declaration, "(")
			declaration = declaration[leftparenIndex:]

			// 替换interface{} to any
			declaration = InterfaceToAnyRegep.ReplaceAllString(declaration, "any")

			doc := theFunc.Doc
			// 删除CRLF
			doc = strings.ReplaceAll(doc, "\r", "")
			doc = strings.ReplaceAll(doc, "\n", "")

			/*
				AutoCompletion
			*/
			var paramAutoCompletionStr string
			if theFunc.Decl != nil && theFunc.Decl.Type != nil && theFunc.Decl.Type.Params != nil {
				var (
					variadic bool
					params   []string
				)

				for index, param := range theFunc.Decl.Type.Params.List {
					_ = index
					if _, ok := param.Type.(*ast.Ellipsis); ok {
						variadic = ok
					}

					var name string
					for _, i := range param.Names {
						name += i.Name
					}

					start, end := param.Type.Pos(), param.Type.End()
					if start-1 < 0 {
						start = 0
					}
					if end-1 < 0 {
						end = 0
					}
					typeVerbose := b[start-1 : end-1]
					/*
						vscode 参数补全格式为： ${n:default}
						n 代表第几个光标：从1开始，0为末尾
						default 为默认补充的值
					*/
					if variadic {
						params = append(params, fmt.Sprintf("${%v:%v...}", index+1, name))
					} else {
						if fixedType := shrinkTypeVerboseName(string(typeVerbose)); fixedType == "any" {
							params = append(params, fmt.Sprintf("${%v:%v}", index+1, name))
						} else {
							params = append(params, fmt.Sprintf("${%v:%v /*type: %v*/}", index+1, name, fixedType))
						}
					}
				}

				paramAutoCompletionStr = strings.Join(params, ", ")
			}

			if overideName != "" {
				return doc, declaration, fmt.Sprintf("%v(%v)", overideName, paramAutoCompletionStr)
			}
			return doc, declaration, fmt.Sprintf("%v(%v)", funcName, paramAutoCompletionStr)
		}
	}

	// 特殊处理
	if declaration == "" {
		declaration = fmt.Sprintf("%#v", f)
		// 去除地址
		addressIndex := strings.LastIndexByte(declaration, '(')
		declaration = declaration[:addressIndex]
		// 去除左右括号
		declaration = strings.TrimLeft(declaration, "(")
		if strings.HasSuffix(declaration, ")") {
			declaration = declaration[:len(declaration)-1]
		}
		// 去除func
		funcIndex := strings.Index(declaration, "func ")
		declaration = declaration[funcIndex+5:]

		// 替换interface{} to any
		declaration = InterfaceToAnyRegep.ReplaceAllString(declaration, "any")
	}
	return
}
