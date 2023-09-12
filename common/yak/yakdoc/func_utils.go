package yakdoc

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

type CachePackage struct {
	fset       *token.FileSet
	pkg        *doc.Package
	parsedFile *ast.File
}

var (
	InterfaceToAnyRegep, _ = regexp.Compile(`interface\s*\{\}`)
	cacheDoc               = make(map[string]*CachePackage) // filename -> CachePackage
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

func handleParams(buf []byte, typ *ast.FuncType) (params []*Field) {
	if typ.Params == nil {
		return nil
	}
	params = make([]*Field, 0, len(typ.Params.List))

	for _, field := range typ.Params.List {
		start, end := field.Type.Pos(), field.Type.End()
		if start-1 < 0 {
			start = 0
		}
		if end-1 < 0 {
			end = 0
		}
		typeVerbose := buf[start-1 : end-1]

		var totalName string
		for _, name := range field.Names {
			totalName += name.Name
			params = append(params, &Field{
				Name: name.Name,
				Type: string(typeVerbose),
			})
		}
	}
	return params
}

func handleResults(buf []byte, typ *ast.FuncType) (results []*Field) {
	if typ.Results == nil {
		return nil
	}
	results = make([]*Field, 0, len(typ.Results.List))

	for _, field := range typ.Results.List {
		start, end := field.Type.Pos(), field.Type.End()
		if start-1 < 0 {
			start = 0
		}
		if end-1 < 0 {
			end = 0
		}
		typeVerbose := buf[start-1 : end-1]
		fixedType := shrinkTypeVerboseName(string(typeVerbose))

		var totalName string
		for _, name := range field.Names {
			totalName += name.Name
			results = append(results, &Field{
				Name: name.Name,
				Type: fixedType,
			})
		}
		// 处理没有实名返回值的情况
		if len(field.Names) == 0 {
			results = append(results, &Field{
				Name: "",
				Type: fixedType,
			})
		}
	}
	return results
}

func customHandleParamsAndResults(libName string, overideName string, params []*Field, results []*Field) ([]*Field, []*Field) {
	// eval时丢掉第一个参数，因为第一个参数是context，是在执行时自动注入的
	if libName == "__GLOBAL__" && overideName == "eval" {
		params = params[1:]
	}
	return params, results
}

// Get description and declaration of a func
func funcDescriptionAndDeclaration(f interface{}, libName string, overideName string) (*FuncDecl, error) {
	fv := reflect.ValueOf(f)
	if fv.Kind() != reflect.Func {
		return nil, fmt.Errorf("not a function")
	}
	pc := fv.Pointer()
	if pc == 0 {
		return nil, fmt.Errorf("cannot get function pointer")
	}
	function := runtime.FuncForPC(pc)
	if function == nil {
		return nil, fmt.Errorf("cannot get function from runtime")
	}

	var (
		cachePkg  *CachePackage
		parsedAst *ast.File
		docPkg    *doc.Package
		fset      *token.FileSet
		ok        bool
		err       error

		declaration            string
		document               string
		paramAutoCompletionStr string
		completeStrs           []string
		params                 []*Field
		results                []*Field
	)

	fileName, line := function.FileLine(0)
	funcName := funcName(f)

	if cachePkg, ok = cacheDoc[fileName]; !ok {
		fset = token.NewFileSet()

		// Parse src
		parsedAst, err = parser.ParseFile(fset, fileName, nil, parser.ParseComments|parser.AllErrors)
		if err != nil {
			return nil, utils.Errorf("parse source file error: %v", err)
		}

		pkg := &ast.Package{
			Name:  "Any",
			Files: make(map[string]*ast.File),
		}
		pkg.Files[fileName] = parsedAst

		importPath, _ := filepath.Abs(fileName)
		docPkg = doc.New(pkg, importPath, doc.AllDecls)

		cacheDoc[fileName] = &CachePackage{
			fset:       fset,
			pkg:        docPkg,
			parsedFile: parsedAst,
		}
	} else {
		fset = cachePkg.fset
		docPkg = cachePkg.pkg
		parsedAst = cachePkg.parsedFile
	}

	found := false

	buf, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	funcs := docPkg.Funcs
	for _, theType := range docPkg.Types {
		funcs = append(funcs, theType.Funcs...)
	}

	for _, theFunc := range funcs {
		if theFunc.Name != funcName {
			continue
		}
		found = true

		decl := theFunc.Decl
		// 获取函数注释
		document = theFunc.Doc
		// 删除CRLF
		document = strings.ReplaceAll(document, "\r", "")
		document = strings.ReplaceAll(document, "\n", "")

		// 获取参数
		if decl != nil && decl.Type != nil && decl.Type.Params != nil {
			params = handleParams(buf, decl.Type)
		}

		// 获取返回值
		if decl != nil && decl.Type != nil && decl.Type.Results != nil {
			results = handleResults(buf, decl.Type)
		}

		break
	}

	// 试图找到map里的
	if !found {
		for _, v := range docPkg.Vars {
			decl := v.Decl
			if decl == nil {
				continue
			}
			if len(decl.Specs) == 0 {
				continue
			}
			iSpec := decl.Specs[0]
			spec, ok := iSpec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if len(spec.Values) == 0 {
				continue
			}
			iValue := spec.Values[0]
			value, ok := iValue.(*ast.CompositeLit)
			if !ok {
				continue
			}

			iType := value.Type
			_, ok = iType.(*ast.MapType)
			if !ok {
				continue
			}
			for _, elt := range value.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.BasicLit)
				if !ok {
					continue
				}
				keyStr, err := strconv.Unquote(key.Value)
				if err != nil {
					continue
				}
				if strings.ToLower(keyStr) != strings.ToLower(funcName) && keyStr != overideName {
					continue
				}

				// 处理 "asd" => 引用其他函数的情况
				v, ok := kv.Value.(*ast.Ident)
				if ok {
					obj := v.Obj
					if obj == nil {
						continue
					}
					decl := obj.Decl
					if decl == nil {
						continue
					}
					if funcDecl, ok := decl.(*ast.FuncDecl); ok {
						params = handleParams(buf, funcDecl.Type)
						results = handleResults(buf, funcDecl.Type)
						found = true
						break
					}

					// 处理 "asd" => 引用变量 => 函数情况
					if specs, ok := decl.(*ast.ValueSpec); ok {
						if len(specs.Values) == 0 {
							continue
						}
						funcLit, ok := specs.Values[0].(*ast.FuncLit)
						if ok {
							params = handleParams(buf, funcLit.Type)
							results = handleResults(buf, funcLit.Type)
							found = true
							break
						}
					}
				}

				// 处理 "asd" => 匿名函数的情况
				funcLit, ok := kv.Value.(*ast.FuncLit)
				if ok {
					params = handleParams(buf, funcLit.Type)
					results = handleResults(buf, funcLit.Type)
					found = true
					break
				}

				// 处理调用函数获得函数的情况
				callExpr, ok := kv.Value.(*ast.CallExpr)
				if !ok {
					continue
				}
				fun, ok := callExpr.Fun.(*ast.Ident)
				if !ok {
					continue
				}
				obj := fun.Obj
				if obj == nil {
					continue
				}
				decl := obj.Decl
				if decl == nil {
					continue
				}
				if funcDecl, ok := decl.(*ast.FuncDecl); ok {
					params = handleParams(buf, funcDecl.Type)
					results = handleResults(buf, funcDecl.Type)
					found = true
					break
				}

				// 按理来说不应该出现 "asd" => utils.xxx的情况，因为上面已经处理了，出现的情况可能是因为重名了

				_ = decl
			}

			if found {
				break
			}
		}
	}

	// 最后的fallback，无法拿到变量名与返回名,尝试直接解析字符串
	if !found {
		lines := strings.Split(string(buf), "\n")
		if line >= len(lines) {
			return nil, fmt.Errorf("line out of range")
		}
		lineStr := lines[line-1]
		// 去除注释
		if commentIndex := strings.Index(lineStr, "//"); commentIndex != -1 {
			lineStr = lineStr[:commentIndex]
		}
		// 去除空格
		lineStr = strings.TrimSpace(lineStr)
		// 去除return
		lineStr = strings.TrimPrefix(lineStr, "return ")
		// 去除func
		lineStr = strings.TrimPrefix(lineStr, "func")
		// 去除空格
		lineStr = strings.TrimSpace(lineStr)
		// 去除左花括号
		index := strings.Index(lineStr, "{")
		if index != -1 {
			lineStr = lineStr[:index]
		}
		// 获取参数
		if paramsIndex := strings.Index(lineStr, "("); paramsIndex != -1 {
			paramsStr := lineStr[paramsIndex+1:]
			paramsEndIndex := strings.Index(paramsStr, ")")
			if paramsEndIndex != -1 {
				paramsStr = paramsStr[:paramsEndIndex]
			}

			paramsStr = strings.TrimRight(paramsStr, ")")
			paramsStr = strings.TrimSpace(paramsStr)
			paramsStrs := strings.Split(paramsStr, ",")
			for i, r := range paramsStrs {
				r = strings.TrimSpace(r)
				if r == "" {
					continue
				}
				splited := strings.Split(r, " ")
				if len(splited) < 2 {
					params = append(params, &Field{
						Name: fmt.Sprintf("v%d", i+1),
						Type: splited[0],
					})
				} else {
					params = append(params, &Field{
						Name: splited[0],
						Type: splited[len(splited)-1],
					})
				}
			}
			paramsEndIndex = strings.Index(lineStr, ")")
			if paramsEndIndex != -1 {
				lineStr = lineStr[paramsEndIndex+2:]
			}
		}
		// 获取返回值
		if resultsIndex := strings.Index(lineStr, "("); resultsIndex != -1 {
			// 多返回值
			resultsStr := lineStr[resultsIndex+1:]
			resultEndIndex := strings.Index(resultsStr, ")")
			if resultEndIndex != -1 {
				resultsStr = resultsStr[:resultEndIndex]
			}
			resultsStr = strings.TrimRight(resultsStr, ")")
			resultsStr = strings.TrimSpace(resultsStr)
			resultsStrs := strings.Split(resultsStr, ",")
			for i, r := range resultsStrs {
				r = strings.TrimSpace(r)
				if r == "" {
					continue
				}
				splited := strings.Split(r, " ")
				if len(splited) < 2 {
					results = append(results, &Field{
						Name: fmt.Sprintf("r%d", i+1),
						Type: splited[0],
					})
				} else {
					results = append(results, &Field{
						Name: splited[0],
						Type: splited[len(splited)-1],
					})
				}
			}
		} else {
			// 单返回值
			resultsStr := strings.TrimSpace(lineStr)
			results = append(results, &Field{
				Name: "r1",
				Type: resultsStr,
			})
		}
	}

	finalName := overideName
	if finalName == "" {
		finalName = funcName
	}

	// 特殊处理params和results
	params, results = customHandleParamsAndResults(libName, overideName, params, results)

	// 通用处理params和results
	completeStrs = make([]string, 0, len(params))
	for i, p := range params {
		variadic := strings.HasPrefix(p.Type, "...")
		_ = variadic
		if p.Name == "" {
			p.Name = fmt.Sprintf("v%d", i+1)
		}
		p.Type = shrinkTypeVerboseName(p.Type)

		/*
			设置vscode AutoCompletion
			vscode 参数补全格式为： ${n:default}
			n 代表第几个光标：从1开始，0为末尾
			default 为默认补充的值
		*/
		if variadic {
			completeStrs = append(completeStrs, fmt.Sprintf("${%v:%v...}", i+1, p.Name))
		} else {
			if p.Type == "any" {
				completeStrs = append(completeStrs, fmt.Sprintf("${%v:%v}", i+1, p.Name))
			} else {
				completeStrs = append(completeStrs, fmt.Sprintf("${%v:%v /*type: %v*/}", i+1, p.Name, p.Type))
			}
		}
	}
	for i, r := range results {
		if r.Name == "" {
			results[i].Name = fmt.Sprintf("r%d", i+1)
		}
		r.Type = shrinkTypeVerboseName(r.Type)
	}

	// 生成declaration
	paramStr := strings.Join(lo.Map(params, func(p *Field, _ int) string {
		return fmt.Sprintf("%s %s", p.Name, p.Type)
	}), ", ")
	resultStr := ""
	if len(results) == 1 {
		if results[0].Name == "r1" {
			resultStr = results[0].Type
		} else {
			resultStr = fmt.Sprintf("(%s %s)", results[0].Name, results[0].Type)
		}
	} else if len(results) > 0 {
		resultStr = fmt.Sprintf("(%s)", strings.Join(lo.Map(results, func(r *Field, i int) string {
			if r.Name == fmt.Sprintf("r%d", i+1) {
				return r.Type
			}
			return fmt.Sprintf("%s %s", r.Name, r.Type)
		}), ", "))
	}
	declaration = fmt.Sprintf("%s(%s) %s", finalName, paramStr, resultStr)
	declaration = strings.TrimSpace(declaration)

	// 生成vscode参数补全
	paramAutoCompletionStr = strings.Join(completeStrs, ", ")
	paramAutoCompletionStr = fmt.Sprintf("%v(%v)", finalName, paramAutoCompletionStr)

	return &FuncDecl{
		LibName:        libName,
		MethodName:     finalName,
		Document:       document,
		Decl:           declaration,
		Params:         params,
		Results:        results,
		VSCodeSnippets: paramAutoCompletionStr,
	}, nil
}
