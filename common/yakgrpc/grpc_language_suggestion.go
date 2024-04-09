package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	pta "github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	stringBuiltinMethod = yakvm.GetStringBuildInMethod()
	bytesBuiltinMethod  = yakvm.GetBytesBuildInMethod()
	mapBuiltinMethod    = yakvm.GetMapBuildInMethod()
	sliceBuiltinMethod  = yakvm.GetSliceBuildInMethod()

	stringBuiltinMethodSuggestionMap = make(map[string]*ypb.SuggestionDescription, len(stringBuiltinMethod))
	bytesBuiltinMethodSuggestionMap  = make(map[string]*ypb.SuggestionDescription, len(bytesBuiltinMethod))
	mapBuiltinMethodSuggestionMap    = make(map[string]*ypb.SuggestionDescription, len(mapBuiltinMethod))
	sliceBuiltinMethodSuggestionMap  = make(map[string]*ypb.SuggestionDescription, len(sliceBuiltinMethod))
	stringBuiltinMethodSuggestions   = make([]*ypb.SuggestionDescription, 0, len(stringBuiltinMethod))
	bytesBuiltinMethodSuggestions    = make([]*ypb.SuggestionDescription, 0, len(bytesBuiltinMethod))
	mapBuiltinMethodSuggestions      = make([]*ypb.SuggestionDescription, 0, len(mapBuiltinMethod))
	sliceBuiltinMethodSuggestions    = make([]*ypb.SuggestionDescription, 0, len(sliceBuiltinMethod))

	yakKeywords = []string{
		"break", "case", "continue", "default", "defer", "else",
		"for", "go", "if", "range", "return", "select", "switch",
		"chan", "func", "fn", "def", "var", "nil", "undefined",
		"map", "class", "include", "type", "bool", "true", "false",
		"string", "try", "catch", "finally", "in",
	}

	standardLibrarySuggestions = make([]*ypb.SuggestionDescription, 0, len(doc.DefaultDocumentHelper.Libs))
	yakKeywordSuggestions      = make([]*ypb.SuggestionDescription, 0)
)

func getLanguageKeywordSuggestions() []*ypb.SuggestionDescription {
	// 懒加载
	if len(yakKeywordSuggestions) == 0 {
		for _, keyword := range yakKeywords {
			yakKeywordSuggestions = append(yakKeywordSuggestions, &ypb.SuggestionDescription{
				Label:       keyword,
				InsertText:  keyword,
				Description: "Language Keyword",
				Kind:        "Keyword",
			})
		}
	}

	return yakKeywordSuggestions
}

func getStringBuiltinMethodSuggestions() []*ypb.SuggestionDescription {
	// 懒加载
	if len(stringBuiltinMethodSuggestionMap) == 0 {
		for methodName, method := range stringBuiltinMethod {
			snippets, _ := method.VSCodeSnippets()
			sug := &ypb.SuggestionDescription{
				Label:       methodName,
				Description: method.Description,
				InsertText:  snippets,
				Kind:        "Method",
			}
			stringBuiltinMethodSuggestionMap[methodName] = sug
			stringBuiltinMethodSuggestions = append(stringBuiltinMethodSuggestions, sug)
		}
	}

	return stringBuiltinMethodSuggestions
}

func getBytesBuiltinMethodSuggestions() []*ypb.SuggestionDescription {
	// 懒加载
	if len(bytesBuiltinMethodSuggestionMap) == 0 {
		for methodName, method := range bytesBuiltinMethod {
			snippets, _ := method.VSCodeSnippets()
			sug := &ypb.SuggestionDescription{
				Label:       methodName,
				Description: method.Description,
				InsertText:  snippets,
				Kind:        "Method",
			}
			bytesBuiltinMethodSuggestionMap[methodName] = sug
			bytesBuiltinMethodSuggestions = append(bytesBuiltinMethodSuggestions, sug)
		}
	}

	return bytesBuiltinMethodSuggestions
}

func getMapBuiltinMethodSuggestions() []*ypb.SuggestionDescription {
	// 懒加载
	if len(mapBuiltinMethodSuggestionMap) == 0 {
		for methodName, method := range mapBuiltinMethod {
			snippets, _ := method.VSCodeSnippets()
			sug := &ypb.SuggestionDescription{
				Label:       methodName,
				Description: method.Description,
				InsertText:  snippets,
				Kind:        "Method",
			}
			mapBuiltinMethodSuggestionMap[methodName] = sug
			mapBuiltinMethodSuggestions = append(mapBuiltinMethodSuggestions, sug)
		}
	}

	return mapBuiltinMethodSuggestions
}

func getSliceBuiltinMethodSuggestions() []*ypb.SuggestionDescription {
	// 懒加载
	if len(sliceBuiltinMethodSuggestionMap) == 0 {
		for methodName, method := range sliceBuiltinMethod {
			snippets, verbose := method.VSCodeSnippets()
			sug := &ypb.SuggestionDescription{
				Label:             methodName,
				DefinitionVerbose: verbose,
				Description:       method.Description,
				InsertText:        snippets,
				Kind:              "Method",
			}
			sliceBuiltinMethodSuggestionMap[methodName] = sug
			sliceBuiltinMethodSuggestions = append(sliceBuiltinMethodSuggestions, sug)
		}
	}

	return sliceBuiltinMethodSuggestions
}

func getStandardLibrarySuggestions() []*ypb.SuggestionDescription {
	// 懒加载
	if len(standardLibrarySuggestions) == 0 {
		for libName := range doc.DefaultDocumentHelper.Libs {
			standardLibrarySuggestions = append(standardLibrarySuggestions, &ypb.SuggestionDescription{
				Label:       libName,
				InsertText:  libName,
				Description: "Standard Library",
				Kind:        "Module",
			})
		}
	}

	return standardLibrarySuggestions
}

func getVscodeSnippetsBySSAValue(funcName string, v *ssaapi.Value) string {
	snippet := funcName
	fun, ok := ssa.ToFunction(ssaapi.GetBareNode(v))
	if !ok {
		return snippet
	}
	funTyp, ok := ssa.ToFunctionType(fun.GetType())
	lenOfParams := len(funTyp.Parameter)
	if !ok {
		return snippet
	}
	snippet += "("
	snippet += strings.Join(
		lo.Map(funTyp.Parameter, func(typ ssa.Type, i int) string {
			if i == lenOfParams-1 && funTyp.IsVariadic {
				typStr := typ.String()
				typStr = strings.TrimLeft(typStr, "[]")
				return fmt.Sprintf("${%d:...%s}", i+1, typStr)
			}
			return fmt.Sprintf("${%d:%s}", i+1, typ)
		}),
		", ",
	)
	snippet += ")"

	return snippet
}

func getFuncDeclByName(name string) *yakdoc.FuncDecl {
	libName, funcName := "", name
	if strings.Contains(name, ".") {
		splited := strings.Split(name, ".")
		libName, funcName = splited[0], splited[1]
	}

	funcDecls := doc.DefaultDocumentHelper.Functions
	if libName != "" {
		lib, ok := doc.DefaultDocumentHelper.Libs[libName]
		if !ok {
			return nil
		}
		funcDecls = lib.Functions
	}

	funcDecl, ok := funcDecls[funcName]
	if ok {
		return funcDecl
	}

	return nil
}

func getInstanceByName(name string) *yakdoc.LibInstance {
	libName, instanceName := "", name
	if strings.Contains(name, ".") {
		splited := strings.Split(name, ".")
		libName, instanceName = splited[0], splited[1]
	}

	instances := doc.DefaultDocumentHelper.Instances

	if libName != "" {
		lib, ok := doc.DefaultDocumentHelper.Libs[libName]
		if !ok {
			return nil
		}
		instances = lib.Instances
	}
	instance, ok := instances[instanceName]
	if ok {
		return instance
	}

	return nil
}

func getGolangTypeStringBySSAType(typ ssa.Type) string {
	typStr := typ.PkgPathString()
	if typStr == "" {
		typStr = typ.String()
	}
	return getGolangTypeStringByTypeStr(typStr)
}

func getGolangTypeStringByTypeStr(typStr string) string {
	switch typStr {
	case "boolean":
		return "bool"
	case "bytes":
		return "[]byte"
	}
	return typStr
}

func shouldExport(key string) bool {
	return (key[0] >= 'A' && key[0] <= 'Z')
}

func getFuncDeclDesc(funcDecl *yakdoc.FuncDecl, typStr string) string {
	document := funcDecl.Document
	if document != "" {
		document = "\n\n" + document
	}
	desc := fmt.Sprintf("```go\nfunc %s\n```%s", funcDecl.Decl, document)
	desc = strings.Replace(desc, "func(", typStr+"(", 1)
	desc = yakdoc.ShrinkTypeVerboseName(desc)
	return desc
}

func getConstInstanceDesc(instance *yakdoc.LibInstance) string {
	desc := fmt.Sprintf("```go\nconst %s = %s\n```", instance.InstanceName, instance.ValueStr)
	desc = yakdoc.ShrinkTypeVerboseName(desc)
	return desc
}

func getFuncTypeDesc(funcTyp *ssa.FunctionType, funcName string) string {
	lenOfParams := len(funcTyp.Parameter)
	desc := fmt.Sprintf("func %s(%s) %s", funcName, strings.Join(lo.Map(
		funcTyp.Parameter, func(typ ssa.Type, i int) string {
			if i == lenOfParams-1 && funcTyp.IsVariadic {
				typStr := typ.String()
				typStr = strings.TrimLeft(typStr, "[]")
				return fmt.Sprintf("r%d ...%s", i+1, typStr)
			}
			return fmt.Sprintf("r%d %s", i+1, typ)
		}),
		", "), funcTyp.ReturnType)
	desc = yakdoc.ShrinkTypeVerboseName(desc)
	return desc
}

func getInstancesAndFuncDecls(word string, containPoint bool) (map[string]*yakdoc.LibInstance, map[string]*yakdoc.FuncDecl) {
	if containPoint {
		libName := strings.Split(word, ".")[0]
		lib, ok := doc.DefaultDocumentHelper.Libs[libName]
		if !ok {
			return nil, nil
		}
		return lib.Instances, lib.Functions
	} else {
		return nil, doc.DefaultDocumentHelper.Functions
	}
}

func getFuncDescByDecls(funcDecls map[string]*yakdoc.FuncDecl, callback func(decl *yakdoc.FuncDecl) string) string {
	desc := ""
	methodNames := utils.GetSortedMapKeys(funcDecls)

	for _, methodName := range methodNames {
		desc += callback(funcDecls[methodName])
	}

	return desc
}

func getFuncDescBytypeStr(typStr string, typName string, isStruct, tab bool) string {
	lib, ok := doc.DefaultDocumentHelper.StructMethods[typStr]
	if !ok {
		return ""
	}

	return getFuncDescByDecls(lib.Functions, func(decl *yakdoc.FuncDecl) string {
		funcDesc := ""
		if isStruct {
			funcDesc = fmt.Sprintf("func (%s) %s\n", typName, strings.TrimPrefix(decl.Decl, "func"))
		} else {
			funcDesc = decl.Decl + "\n"
		}
		if tab {
			funcDesc = "    " + funcDesc
		}
		return funcDesc
	})
}

func getBuiltinFuncDeclAndDoc(name string, bareTyp ssa.Type) (desc string, doc string) {
	var m map[string]*ypb.SuggestionDescription

	switch bareTyp.GetTypeKind() {
	case ssa.SliceTypeKind:
		// []byte / [] 内置方法
		rTyp, ok := bareTyp.(*ssa.ObjectType)
		if !ok {
			break
		}
		if rTyp.KeyTyp.GetTypeKind() == ssa.BytesTypeKind {
			getBytesBuiltinMethodSuggestions()
			m = bytesBuiltinMethodSuggestionMap
		} else {
			getSliceBuiltinMethodSuggestions()
			m = sliceBuiltinMethodSuggestionMap
		}
	case ssa.MapTypeKind:
		// map 内置方法
		getMapBuiltinMethodSuggestions()
		m = mapBuiltinMethodSuggestionMap
	case ssa.StringTypeKind:
		// string 内置方法
		getStringBuiltinMethodSuggestions()
		m = stringBuiltinMethodSuggestionMap
	}
	sug, ok := m[name]
	if ok {
		desc := sug.Label
		if sug.DefinitionVerbose != "" {
			desc = sug.DefinitionVerbose
		}
		return desc, sug.Description
	}
	return
}

func getFuncDeclAndDocBySSAValue(name string, v *ssaapi.Value) (desc string, document string) {
	// 标准库函数
	funcDecl := getFuncDeclByName(name)
	if funcDecl != nil {
		return yakdoc.ShrinkTypeVerboseName(funcDecl.Decl), funcDecl.Document
	}

	lastName := name
	if strings.Contains(lastName, ".") {
		lastName = lastName[strings.LastIndex(lastName, ".")+1:]
	}

	// 结构体 / 接口方法
	bareTyp := ssaapi.GetBareType(v.GetType())
	typStr := getGolangTypeStringBySSAType(bareTyp)
	lib, ok := doc.DefaultDocumentHelper.StructMethods[typStr]
	if ok {
		funcDecl, ok = lib.Functions[lastName]
		if ok {
			return yakdoc.ShrinkTypeVerboseName(funcDecl.Decl), funcDecl.Document
		}
	}
	// 用户自定义函数
	if bareTyp.GetTypeKind() == ssa.FunctionTypeKind {
		funcTyp, ok := ssa.ToFunctionType(bareTyp)
		if ok {
			desc = getFuncTypeDesc(funcTyp, lastName)
			return
		}
	}

	// 类型内置方法
	desc, document = getBuiltinFuncDeclAndDoc(lastName, bareTyp)
	if desc != "" {
		return
	}

	return
}

func getExternLibDesc(name, typName string) string {
	// 标准库
	lib, ok := doc.DefaultDocumentHelper.Libs[name]
	if !ok {
		// break
		return ""
	}

	var builder strings.Builder
	// desc :=
	// desc = yakdoc.ShrinkTypeVerboseName(desc)

	builder.WriteString(fmt.Sprintf("```go\npackage %s\n\n", name))
	instanceKeys := utils.GetSortedMapKeys(lib.Instances)
	for _, key := range instanceKeys {
		instance := lib.Instances[key]
		builder.WriteString(yakdoc.ShrinkTypeVerboseName(fmt.Sprintf("const %s %s = %s\n", instance.InstanceName, getGolangTypeStringByTypeStr(instance.Type), instance.ValueStr)))
	}
	builder.WriteRune('\n')
	builder.WriteString(getFuncDescByDecls(lib.Functions, func(decl *yakdoc.FuncDecl) string {
		return yakdoc.ShrinkTypeVerboseName(fmt.Sprintf("func %s\n", decl.Decl))
	}))
	builder.WriteString("\n```")
	return builder.String()
}

func getDescFromSSAValue(name string, v *ssaapi.Value) string {
	bareTyp := ssaapi.GetBareType(v.GetType())
	typStr := getGolangTypeStringBySSAType(bareTyp)
	typName := typStr
	desc := ""
	if strings.Contains(typName, ".") {
		typName = typName[strings.LastIndex(typName, ".")+1:]
	}
	nameContainsPoint := strings.Contains(name, ".")

	if !nameContainsPoint {
		switch bareTyp.GetTypeKind() {
		case ssa.FunctionTypeKind:
			// 标准库函数
			funcDecl := getFuncDeclByName(name)
			if funcDecl != nil {
				desc = getFuncDeclDesc(funcDecl, typStr)
				break
			}
			// 用户自定义函数
			funcTyp, ok := ssa.ToFunctionType(bareTyp)
			if !ok {
				break
			}

			lastName := name
			if strings.Contains(lastName, ".") {
				lastName = lastName[strings.LastIndex(lastName, ".")+1:]
			}
			desc = fmt.Sprintf("```go\n%s\n```", getFuncTypeDesc(funcTyp, lastName))
		case ssa.StructTypeKind:
			rTyp, ok := bareTyp.(*ssa.ObjectType)
			if !ok {
				break
			}
			if rTyp.Combination {
				desc = fmt.Sprintf("```go\n%s (%s)\n```", name, typStr)
				break
			}
			desc = fmt.Sprintf("```go\ntype %s struct {\n", typName)
			for _, key := range rTyp.Keys {
				// 过滤掉非导出字段
				if !shouldExport(key.String()) {
					continue
				}
				fieldType := rTyp.GetField(key)
				desc += fmt.Sprintf("    %-20s %s\n", key, getGolangTypeStringBySSAType(fieldType))
			}
			desc += "}"
			methodDescriptions := getFuncDescBytypeStr(typStr, typName, true, false)
			if methodDescriptions != "" {
				desc += "\n\n"
				desc += methodDescriptions
			}
			desc += "\n```"
		case ssa.InterfaceTypeKind:
			desc = fmt.Sprintf("```go\ntype %s interface {\n", typName)
			methodDescriptions := getFuncDescBytypeStr(typStr, typName, false, true)
			desc += methodDescriptions
			desc += "}"
			desc += "\n```"
		case ssa.AnyTypeKind:
			desc = getExternLibDesc(name, typName)
		}
	} else {
		// ! 这里可能存在value实际上是parent 而不是其本身
		lastName := name[strings.LastIndex(name, ".")+1:]
		if v.IsExternLib() {
			// 标准库函数
			funcDecl := getFuncDeclByName(name)
			if funcDecl != nil {
				desc = getFuncDeclDesc(funcDecl, lastName)
			}
			// 标准库常量
			instance := getInstanceByName(name)
			if instance != nil {
				desc = getConstInstanceDesc(instance)
			}
		} else {
			// 结构体 / 接口方法
			lib, ok := doc.DefaultDocumentHelper.StructMethods[typStr]
			if ok {
				funcDecl, ok := lib.Functions[lastName]
				if ok {
					desc = getFuncDeclDesc(funcDecl, lastName)
				} else {
					instance, ok := lib.Instances[lastName]
					if ok {
						desc = yakdoc.ShrinkTypeVerboseName(fmt.Sprintf("```go\nfield %s %s\n```", instance.InstanceName, getGolangTypeStringByTypeStr(instance.Type)))
					}
				}
			} else {
				// 内置方法
				decl, document := getBuiltinFuncDeclAndDoc(lastName, bareTyp)
				desc = fmt.Sprintf("```go\nfunc %s\n```\n\n%s", decl, document)
			}
		}
	}

	if desc == "" && !nameContainsPoint {
		desc = fmt.Sprintf("```go\ntype %s %s\n```", name, typStr)
	}
	return desc
}

func sortValuesByPosition(values ssaapi.Values, position *ssa.Range) ssaapi.Values {
	// todo: 需要修改SSA，需要真正的RefLocation
	values = values.Filter(func(v *ssaapi.Value) bool {
		position2 := v.GetRange()
		if position2 == nil {
			return false
		}
		if position2.Start.Line > position.Start.Line {
			return false
		}
		return true
	})
	sort.SliceStable(values, func(i, j int) bool {
		line1, line2 := values[i].GetRange().Start.Line, values[j].GetRange().Start.Line
		if line1 == line2 {
			return values[i].GetRange().Start.Column > values[j].GetRange().Start.Column
		} else {
			return line1 > line2
		}
	})
	return values
}

func getSSAParentValueByPosition(prog *ssaapi.Program, sourceCode string, position *ssa.Range) *ssaapi.Value {
	word := strings.Split(sourceCode, ".")[0]
	values := prog.Ref(word).Filter(func(v *ssaapi.Value) bool {
		position2 := v.GetRange()
		if position2 == nil {
			return false
		}
		if position2.Start.Line > position.Start.Line {
			return false
		}
		return true
	})
	values = sortValuesByPosition(values, position)
	if len(values) == 0 {
		return nil
	}
	return values[0].GetSelf()
}

func getSSAValueByPosition(prog *ssaapi.Program, sourceCode string, position *ssa.Range) *ssaapi.Value {
	var values ssaapi.Values
	for i, word := range strings.Split(sourceCode, ".") {
		if i == 0 {
			values = prog.Ref(word)
		} else {
			// fallback
			newValues := values.Ref(word)
			if len(newValues) == 0 {
				break
			} else {
				values = newValues
			}
		}
	}
	values = sortValuesByPosition(values, position)
	if len(values) == 0 {
		return nil
	}
	return values[0].GetSelf()
}

func trimSourceCode(sourceCode string) (string, bool) {
	containPoint := strings.Contains(sourceCode, ".")
	if strings.HasSuffix(sourceCode, ".") {
		sourceCode = sourceCode[:len(sourceCode)-1]
	}
	return sourceCode, containPoint
}

func OnHover(prog *ssaapi.Program, req *ypb.YaklangLanguageSuggestionRequest) (ret []*ypb.SuggestionDescription) {
	ret = make([]*ypb.SuggestionDescription, 0)
	position := GrpcRangeToPosition(req.GetRange())
	word, _ := trimSourceCode(*position.SourceCode)
	v := getSSAParentValueByPosition(prog, word, position)
	// fallback
	if v == nil {
		v = getSSAValueByPosition(prog, word, position)
		if v == nil {
			return ret
		}
	}

	ret = append(ret, &ypb.SuggestionDescription{
		Label: getDescFromSSAValue(word, v),
	})

	return ret
}

func OnSignature(prog *ssaapi.Program, req *ypb.YaklangLanguageSuggestionRequest) (ret []*ypb.SuggestionDescription) {
	ret = make([]*ypb.SuggestionDescription, 0)
	position := GrpcRangeToPosition(req.GetRange())
	word, _ := trimSourceCode(*position.SourceCode)
	v := getSSAParentValueByPosition(prog, word, position)
	// fallback
	if v == nil {
		v = getSSAValueByPosition(prog, word, position)
		if v == nil {
			return ret
		}
	}

	desc, doc := getFuncDeclAndDocBySSAValue(word, v)
	if desc != "" {
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       desc,
			Description: doc,
		})
	}

	return ret
}

func OnCompletion(prog *ssaapi.Program, req *ypb.YaklangLanguageSuggestionRequest) (ret []*ypb.SuggestionDescription) {
	ret = make([]*ypb.SuggestionDescription, 0)
	position := GrpcRangeToPosition(req.GetRange())
	word, containPoint := trimSourceCode(*position.SourceCode)
	v := getSSAParentValueByPosition(prog, word, position)
	if !containPoint {
		// 库补全
		ret = append(ret, getStandardLibrarySuggestions()...)
		// 关键字补全
		ret = append(ret, getLanguageKeywordSuggestions()...)
		// 用户自定义变量补全
		for id, values := range prog.GetAllSymbols() {
			// 不应该再补全标准库
			if _, ok := doc.DefaultDocumentHelper.Libs[id]; ok {
				continue
			}
			// 不应该再补全包含.的符号
			if strings.Contains(id, ".") {
				continue
			}
			// todo: 需要更严谨的过滤
			values = values.Filter(func(value *ssaapi.Value) bool {
				position2 := value.GetRange()
				if position2 == nil {
					return false
				}
				line := position2.Start.Line
				if line < position.Start.Line {
					return true
				} else if line == position.Start.Line {
					return id != word
				}
				return false
			})
			if len(values) == 0 {
				continue
			}
			// todo: 需要更严谨的处理
			values = sortValuesByPosition(values, position)
			v := values[0]
			insertText := id
			vKind := "Variable"
			if v.IsFunction() {
				vKind = "Function"
				insertText = getVscodeSnippetsBySSAValue(id, v)
			}
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       id,
				Description: "",
				InsertText:  insertText,
				Kind:        vKind,
			})
		}
	}

	// 库函数补全
	instances, funcDecls := getInstancesAndFuncDecls(word, containPoint)
	if funcDecls != nil {
		for _, decl := range funcDecls {
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       decl.MethodName,
				Description: decl.Document,
				InsertText:  decl.VSCodeSnippets,
				Kind:        "Function",
			})
		}
	}
	// 库常量补全
	if len(instances) > 0 {
		for _, instance := range instances {
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       instance.InstanceName,
				Description: "",
				InsertText:  instance.InstanceName,
				Kind:        "Constant",
			})
		}
	}

	// 结构体成员补全
	if !containPoint {
		return ret
	}

	if v == nil {
		return ret
	}
	bareTyp := ssaapi.GetBareType(v.GetType())
	typStr := getGolangTypeStringBySSAType(bareTyp)
	typKind := bareTyp.GetTypeKind()
	switch typKind {
	case ssa.SliceTypeKind, ssa.TupleTypeKind:
		// []byte / [] 内置方法
		rTyp, ok := bareTyp.(*ssa.ObjectType)
		if !ok {
			break
		}
		if rTyp.KeyTyp.GetTypeKind() == ssa.BytesTypeKind {
			ret = append(ret, getBytesBuiltinMethodSuggestions()...)
		} else {
			ret = append(ret, getSliceBuiltinMethodSuggestions()...)
		}
	case ssa.MapTypeKind:
		// map 内置方法
		ret = append(ret, getMapBuiltinMethodSuggestions()...)

		// map 成员
		filterMap := make(map[string]struct{})
		v.GetUsers().Filter(func(u *ssaapi.Value) bool {
			position2 := u.GetRange()
			if position2 == nil {
				return false
			}
			return u.IsMember() && position2.Start.Line <= position.Start.Line
		}).ForEach(func(v *ssaapi.Value) {
			key := v.GetOperand(1)
			if _, ok := filterMap[key.String()]; ok {
				return
			}
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       key.String(),
				Description: "",
				InsertText:  key.String(),
				Kind:        "Field",
			})
			filterMap[key.String()] = struct{}{}
		})
	case ssa.StringTypeKind:
		// string 内置方法
		ret = append(ret, getStringBuiltinMethodSuggestions()...)
	}

	// 接口方法，结构体成员与方法，定义类型方法
	lib, ok := doc.DefaultDocumentHelper.StructMethods[typStr]
	if !ok {
		return ret
	}

	for _, instance := range lib.Instances {
		// 过滤掉非导出字段
		if !shouldExport(instance.InstanceName) {
			continue
		}
		keyStr := instance.InstanceName
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       keyStr,
			Description: "",
			InsertText:  keyStr,
			Kind:        "Field",
		})
	}

	for methodName, funcDecl := range lib.Functions {
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       methodName,
			Description: funcDecl.Document,
			InsertText:  funcDecl.VSCodeSnippets,
			Kind:        "Method",
		})
	}

	return ret
}

func GrpcRangeToPosition(r *ypb.Range) *ssa.Range {
	// TODO: ypb.Range should have `Offset`
	return ssa.NewRange(
		ssa.NewPosition(0, r.StartLine, r.StartColumn-1),
		ssa.NewPosition(0, r.EndLine, r.EndColumn-1),
		r.Code,
	)
}

func (s *Server) YaklangLanguageSuggestion(ctx context.Context, req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, error) {
	ret := &ypb.YaklangLanguageSuggestionResponse{}
	opt := pta.GetPluginSSAOpt(req.YakScriptType)
	opt = append(opt, ssaapi.WithIgnoreSyntaxError(true))
	prog, err := ssaapi.Parse(req.YakScriptCode, opt...)
	if err != nil {
		return nil, errors.New("ssa parse error")
	}
	// todo: 处理YakScriptType，不同语言的补全、提示可能有不同
	switch req.InspectType {
	case "completion":
		ret.SuggestionMessage = OnCompletion(prog, req)
	case "hover":
		ret.SuggestionMessage = OnHover(prog, req)
	case "signature":
		ret.SuggestionMessage = OnSignature(prog, req)
	}
	return ret, nil
}
