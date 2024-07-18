package yakgrpc

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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

	yakTypes = []string{
		"uint", "uint8", "byte", "uint16", "uint32", "uint64",
		"int", "int8", "int16", "int32", "int64",
		"bool", "float", "float64", "double", "string", "omap", "var",
		"any",
	}

	standardLibrarySuggestions = make([]*ypb.SuggestionDescription, 0, len(doc.DefaultDocumentHelper.Libs))
	yakKeywordSuggestions      = make([]*ypb.SuggestionDescription, 0)
	yakTypeSuggestions         = make([]*ypb.SuggestionDescription, 0)
	progCacheMap               = utils.NewTTLCache[*ssaapi.Program](0)
)

func getLanguageKeywordSuggestions() []*ypb.SuggestionDescription {
	// 懒加载
	if len(yakKeywordSuggestions) == 0 {
		yakKeywordSuggestions = make([]*ypb.SuggestionDescription, 0, len(yakKeywords))
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

func getLanguageBasicTypeSuggestions() []*ypb.SuggestionDescription {
	// 懒加载
	if len(yakTypeSuggestions) == 0 {
		yakTypeSuggestions = make([]*ypb.SuggestionDescription, 0, len(yakTypes))
		for _, typ := range yakTypes {
			yakTypeSuggestions = append(yakTypeSuggestions, &ypb.SuggestionDescription{
				Label:       typ,
				InsertText:  typ,
				Description: "Basic Type",
				Kind:        "Class",
			})
		}
	}

	return yakTypeSuggestions
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

func getFrontValueByOffset(prog *ssaapi.Program, editor *memedit.MemEditor, rng *ssa.Range, skipNum int) *ssaapi.Value {
	// use editor instead of prog.Program.Editor because of ssa cache
	var value ssa.Value
	offset := rng.GetEndOffset()
	for i := 0; i < skipNum; i++ {
		_, offset = prog.Program.SearchIndexAndOffsetByOffset(offset)
		offset--
	}
	_, value = prog.Program.GetFrontValueByOffset(offset)
	if !utils.IsNil(value) {
		return prog.NewValue(value)
	}
	return nil
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

func getFuncDeclByName(libName, funcName string) *yakdoc.FuncDecl {
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

func getInstanceByName(libName, instanceName string) *yakdoc.LibInstance {
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

func getFuncDeclDesc(v *ssaapi.Value, funcDecl *yakdoc.FuncDecl) string {
	document := funcDecl.Document
	if document != "" {
		document = "\n\n" + document
	}
	decl, desc := funcDecl.Decl, ""
	funcName := funcDecl.MethodName

	bareV := ssaapi.GetBareNode(v)
	if f, ok := ssa.ToFunction(bareV); ok {
		if strings.HasPrefix(decl, "func(") {
			// fix decl
			decl = strings.Replace(decl, "func(", funcName+"(", 1)
		}

		funcTyp, ok := ssa.ToFunctionType(f.GetType())
		if ok {
			isMethod := funcTyp.IsMethod
			prefix := ""
			if isMethod && len(funcTyp.Parameter) > 0 {
				prefix = fmt.Sprintf("(%s) ", funcTyp.Parameter[0])
			}

			if f.IsGeneric() {
				offset := 0
				if isMethod {
					offset = 1
				}
				// fix generic function decl
				paramsStr := strings.Join(lo.Map(funcDecl.Params, func(item *yakdoc.Field, index int) string {
					return fmt.Sprintf("%s %s", item.Name, funcTyp.Parameter[index+offset])
				}), ", ")
				var returnsStr string
				if funcTyp.ReturnType.GetTypeKind() == ssa.TupleTypeKind {
					returnTyp, _ := ssa.ToObjectType(funcTyp.ReturnType)

					returnsStr = strings.Join(lo.Map(funcDecl.Results, func(item *yakdoc.Field, index int) string {
						return fmt.Sprintf("%s %s", item.Name, returnTyp.GetField(ssa.NewConst(index)))
					}), ", ")
				} else {
					returnsStr = funcTyp.ReturnType.String()
				}

				desc = fmt.Sprintf("```go\n%s%s(%s) %s\n```%s", prefix, funcName, paramsStr, returnsStr, document)
			}
		}
	}

	if desc == "" {
		desc = fmt.Sprintf("```go\nfunc %s\n```%s", decl, document)
	}

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
	params := funcTyp.Parameter
	if funcTyp.IsMethod {
		lenOfParams--
		if len(params) > 0 {
			params = params[1:]
		}
	}

	paramsStr := lo.Map(
		params, func(typ ssa.Type, i int) string {
			if i == lenOfParams-1 && funcTyp.IsVariadic {
				typStr := typ.String()
				typStr = strings.TrimLeft(typStr, "[]")
				return fmt.Sprintf("i%d ...%s", i+1, typStr)
			}
			return fmt.Sprintf("i%d %s", i+1, typ)
		})
	paramsRaw := strings.Join(paramsStr, ", ")

	var desc string

	if funcTyp.IsMethod && len(funcTyp.Parameter) > 0 {
		desc = fmt.Sprintf("func (%s) %s(%s) %s", funcTyp.Parameter[0], funcName, paramsRaw, funcTyp.ReturnType)
	} else {
		desc = fmt.Sprintf("func %s(%s) %s", funcName,
			paramsRaw,
			funcTyp.ReturnType,
		)
	}
	desc = yakdoc.ShrinkTypeVerboseName(desc)
	return desc
}

func getInstancesAndFuncDecls(v *ssaapi.Value, containPoint bool) (map[string]*yakdoc.LibInstance, map[string]*yakdoc.FuncDecl) {
	if !containPoint || v.IsNil() {
		return nil, doc.DefaultDocumentHelper.Functions
	}

	libName := v.GetName()
	lib, ok := doc.DefaultDocumentHelper.Libs[libName]
	if ok {
		return lib.Instances, lib.Functions
	} else {
		return nil, nil
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
	if utils.IsNil(bareTyp) {
		return
	}

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
	if v.IsNil() {
		return "", ""
	}

	lastName := name
	_, after, ok := strings.Cut(name, ".")
	if ok {
		lastName = after
	}

	var (
		parentBareTyp ssa.Type
		parentTypStr  string
		funcTyp       *ssa.FunctionType
	)
	parentV := v.GetObject()
	if parentV != nil {
		parentBareTyp = ssaapi.GetBareType(parentV.GetType())
		parentTypStr = getGolangTypeStringBySSAType(parentBareTyp)
	}

	bareTyp := ssaapi.GetBareType(v.GetType())
	typKind := bareTyp.GetTypeKind()
	if bareTyp.GetTypeKind() == ssa.FunctionTypeKind {
		funcTyp, _ = ssa.ToFunctionType(bareTyp)
	}

	if v.IsExtern() {
		if typKind == ssa.FunctionTypeKind {
			// 标准库函数
			// value name 里包含了库名与函数名
			libName, lastName, _ := strings.Cut(v.GetName(), ".")
			funcDecl := getFuncDeclByName(libName, lastName)
			if funcDecl != nil {
				return getFuncDeclDesc(v, funcDecl), funcDecl.Document
			}
		}
	}

	// 结构体 / 接口方法
	lib, ok := doc.DefaultDocumentHelper.StructMethods[parentTypStr]
	if ok {
		funcDecl, ok := lib.Functions[lastName]
		if ok {
			return getFuncDeclDesc(v, funcDecl), funcDecl.Document
		}
	}

	// 类型内置方法, 方法签名现在用 SSA Value 获取
	funcObjectType := v.GetFunctionObjectType()
	_, document = getBuiltinFuncDeclAndDoc(lastName, funcObjectType)

	// 用户自定义函数
	if funcTyp != nil {
		desc = getFuncTypeDesc(funcTyp, lastName)
		return
	}

	return
}

func getExternLibDesc(name string) string {
	// 标准库
	lib, ok := doc.DefaultDocumentHelper.Libs[name]
	if !ok {
		// break
		return ""
	}

	var builder strings.Builder

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

func getDescFromSSAValue(name string, containPoint bool, prog *ssaapi.Program, v *ssaapi.Value) string {
	if v.IsNil() {
		return ""
	}

	desc := ""
	lastName := name

	if lastIndex := strings.LastIndex(name, "."); lastIndex != -1 {
		lastName = name[lastIndex+1:]
	}

	varname := v.GetName()
	bareTyp := ssaapi.GetBareType(v.GetType())
	typStr := getGolangTypeStringBySSAType(bareTyp)
	typKind := bareTyp.GetTypeKind()
	shortTypName := typStr
	if strings.Contains(shortTypName, ".") {
		shortTypName = shortTypName[strings.LastIndex(shortTypName, ".")+1:]
	}

	if v.IsExtern() {
		if v.IsExternLib() {
			// 标准库
			desc = getExternLibDesc(varname)
		} else {
			var libName, lastName string
			if strings.Contains(varname, ".") {
				libName, lastName, _ = strings.Cut(varname, ".")
			} else {
				libName, lastName = "", varname
			}
			if typKind == ssa.FunctionTypeKind {
				// 标准库函数
				funcDecl := getFuncDeclByName(libName, lastName)
				if funcDecl != nil {
					desc = getFuncDeclDesc(v, funcDecl)
				}
			} else {
				// 标准库常量
				instance := getInstanceByName(libName, lastName)
				if instance != nil {
					desc = getConstInstanceDesc(instance)
				}
			}
		}

		if desc != "" {
			return desc
		}
	}

	switch typKind {
	case ssa.FunctionTypeKind:
		desc, _ = getFuncDeclAndDocBySSAValue(name, v)
		if !strings.HasPrefix(desc, "```") {
			desc = fmt.Sprintf("```go\n%s\n```", desc)
		}
	case ssa.StructTypeKind:
		rTyp, ok := bareTyp.(*ssa.ObjectType)
		if !ok {
			break
		}
		if rTyp.Combination {
			desc = fmt.Sprintf("```go\n%s (%s)\n```", name, typStr)
			break
		}
		desc = fmt.Sprintf("```go\ntype %s struct {\n", shortTypName)
		for _, key := range rTyp.Keys {
			// 过滤掉非导出字段
			if !shouldExport(key.String()) {
				continue
			}
			fieldType := rTyp.GetField(key)
			desc += fmt.Sprintf("    %-20s %s\n", key, getGolangTypeStringBySSAType(fieldType))
		}
		desc += "}"
		methodDescriptions := getFuncDescBytypeStr(typStr, shortTypName, true, false)
		if methodDescriptions != "" {
			desc += "\n\n"
			desc += methodDescriptions
		}
		desc += "\n```"
	case ssa.InterfaceTypeKind:
		desc = fmt.Sprintf("```go\ntype %s interface {\n", shortTypName)
		methodDescriptions := getFuncDescBytypeStr(typStr, shortTypName, false, true)
		desc += methodDescriptions
		desc += "}"
		desc += "\n```"
	}

	// 结构体成员
	if desc == "" {
		parentV := v.GetObject()
		if parentV != nil {
			parentBareTyp := ssaapi.GetBareType(parentV.GetType())
			parentTypStr := getGolangTypeStringBySSAType(parentBareTyp)
			lib, ok := doc.DefaultDocumentHelper.StructMethods[parentTypStr]
			if ok {
				instance, ok := lib.Instances[lastName]
				if ok {
					desc = yakdoc.ShrinkTypeVerboseName(fmt.Sprintf("```go\nfield %s %s\n```", instance.InstanceName, getGolangTypeStringByTypeStr(instance.Type)))
				}
			}
		}
	}

	if desc == "" {
		desc = fmt.Sprintf("```go\ntype %s %s\n```", lastName, typStr)
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
		if position2.GetStart().GetLine() > position.GetStart().GetLine() {
			return false
		}
		return true
	})
	sort.SliceStable(values, func(i, j int) bool {
		line1, line2 := values[i].GetRange().GetStart().GetLine(), values[j].GetRange().GetStart().GetLine()
		if line1 == line2 {
			return values[i].GetRange().GetStart().GetColumn() > values[j].GetRange().GetStart().GetColumn()
		} else {
			return line1 > line2
		}
	})
	return values
}

// Deprecated: now can get the closest value
func getSSAParentValueByPosition(prog *ssaapi.Program, sourceCode string, position *ssa.Range) *ssaapi.Value {
	word := strings.Split(sourceCode, ".")[0]
	values := prog.Ref(word).Filter(func(v *ssaapi.Value) bool {
		position2 := v.GetRange()
		if position2 == nil {
			return false
		}
		if position2.GetStart().GetLine() > position.GetStart().GetLine() {
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

// Deprecated: now can get the closest value
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

func getFuncCompletionBySSAType(funcName string, typ ssa.Type) string {
	s, ok := ssa.ToFunctionType(typ)
	if !ok {
		return ""
	}

	paras := make([]string, 0, s.ParameterLen)
	for i := 0; i < s.ParameterLen; i++ {
		paramsStr := s.Parameter[i].String()
		if (i == s.ParameterLen-1) && s.IsVariadic {
			paramsStr = "..." + paramsStr
		}
		paras = append(paras, fmt.Sprintf("${%d:%s}", i+1, paramsStr))
	}

	return fmt.Sprintf(
		"%s(%s)",
		funcName,
		strings.Join(
			paras,
			", ",
		),
	)
}

func trimSourceCode(sourceCode string) (string, bool) {
	containPoint := strings.Contains(sourceCode, ".")
	if strings.HasSuffix(sourceCode, ".") {
		sourceCode = sourceCode[:len(sourceCode)-1]
	}
	return strings.TrimSpace(sourceCode), containPoint
}

func OnHover(prog *ssaapi.Program, word string, containPoint bool, rng *ssa.Range, v *ssaapi.Value) (ret []*ypb.SuggestionDescription) {
	ret = append(ret, &ypb.SuggestionDescription{
		Label: getDescFromSSAValue(word, containPoint, prog, v),
	})

	return ret
}

func OnSignature(prog *ssaapi.Program, word string, containPoint bool, rng *ssa.Range, v *ssaapi.Value) (ret []*ypb.SuggestionDescription) {
	ret = make([]*ypb.SuggestionDescription, 0)

	desc, doc := getFuncDeclAndDocBySSAValue(word, v)
	if desc != "" {
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       desc,
			Description: doc,
		})
	}

	return ret
}

func completionYakStandardLibrary() (ret []*ypb.SuggestionDescription) {
	// 库补全
	return getStandardLibrarySuggestions()
}

func completionYakLanguageKeyword() (ret []*ypb.SuggestionDescription) {
	// 关键字补全
	return getLanguageKeywordSuggestions()
}

func completionYakLanguageBasicType() (ret []*ypb.SuggestionDescription) {
	// 基础类型补全
	return getLanguageBasicTypeSuggestions()
}

func completionUserDefinedVariable(prog *ssaapi.Program, rng *ssa.Range) (ret []*ypb.SuggestionDescription) {
	if prog == nil || prog.Program == nil {
		return
	}

	ret = make([]*ypb.SuggestionDescription, 0)
	// 自定义变量补全
	uniqMap := make(map[string]struct{})
	// 需要反转，因为是按 offset 顺序排列的
	for _, item := range lo.Reverse(prog.GetAllOffsetItemsBefore(rng.GetEndOffset())) {
		variable := item.GetVariable()
		varName := variable.GetName()
		if _, ok := uniqMap[varName]; ok {
			continue
		}
		uniqMap[varName] = struct{}{}
		bareValue := item.GetValue()
		v := prog.NewValue(bareValue)

		// 不应该再补全标准库函数和标准库
		if _, ok := doc.DefaultDocumentHelper.Functions[varName]; ok {
			continue
		}
		if _, ok := doc.DefaultDocumentHelper.Libs[varName]; ok {
			continue
		}
		// 不应该再补全包含.或#的符号
		if strings.Contains(varName, ".") || strings.Contains(varName, "#") {
			continue
		}

		insertText := varName
		vKind := "Variable"
		if !v.IsNil() && v.IsFunction() {
			vKind = "Function"
			insertText = getVscodeSnippetsBySSAValue(varName, v)
		}
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       varName,
			Description: "",
			InsertText:  insertText,
			Kind:        vKind,
		})
	}
	return
}

func completionYakGlobalFunctions() (ret []*ypb.SuggestionDescription) {
	ret = make([]*ypb.SuggestionDescription, 0, len(doc.DefaultDocumentHelper.Functions))
	// 全局函数补全
	for funcName, funcDecl := range doc.DefaultDocumentHelper.Functions {
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       funcName,
			Description: funcDecl.Document,
			InsertText:  funcDecl.VSCodeSnippets,
			Kind:        "Function",
		})
	}
	return
}

func completionYakStandardLibraryChildren(v *ssaapi.Value) (ret []*ypb.SuggestionDescription) {
	libName := v.GetName()
	lib, ok := doc.DefaultDocumentHelper.Libs[libName]
	if !ok {
		return
	}
	ret = make([]*ypb.SuggestionDescription, 0, len(lib.Functions)+len(lib.Instances))
	if len(lib.Functions) > 0 {
		for _, decl := range lib.Functions {
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       decl.MethodName,
				Description: decl.Document,
				InsertText:  decl.VSCodeSnippets,
				Kind:        "Function",
			})
		}
	}
	if len(lib.Instances) > 0 {
		for _, instance := range lib.Instances {
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       instance.InstanceName,
				Description: "",
				InsertText:  instance.InstanceName,
				Kind:        "Constant",
			})
		}
	}
	return
}

func completionYakTypeBuiltinMethod(rng *ssa.Range, v *ssaapi.Value, realTyp ...ssa.Type) (ret []*ypb.SuggestionDescription) {
	var bareTyp ssa.Type
	if len(realTyp) > 0 {
		bareTyp = realTyp[0]
	} else {
		bareTyp = ssaapi.GetBareType(v.GetType())
	}

	typKind := bareTyp.GetTypeKind()
	if typKind == ssa.OrTypeKind {
		// or 类型特殊处理
		orTyp, ok := bareTyp.(*ssa.OrType)
		if !ok {
			return
		}
		for _, typ := range orTyp.GetTypes() {
			ret = append(ret, completionYakTypeBuiltinMethod(rng, v, typ)...)
		}
		return
	}

	switch typKind {
	case ssa.BytesTypeKind:
		// []byte 内置方法
		ret = append(ret, getBytesBuiltinMethodSuggestions()...)
	case ssa.SliceTypeKind, ssa.TupleTypeKind:
		ret = append(ret, getSliceBuiltinMethodSuggestions()...)
	case ssa.MapTypeKind:
		// map 内置方法
		ret = append(ret, getMapBuiltinMethodSuggestions()...)

		// map 成员
		for _, slices := range v.GetMembers() {
			key, member := slices[0], slices[1]
			if member.IsUndefined() {
				continue
			}

			kind := "Field"
			insertText := ""
			label := key.String()
			if kind := key.GetTypeKind(); kind == ssa.StringTypeKind || kind == ssa.BytesTypeKind {
				label, _ = strconv.Unquote(label)
			}
			insertText = label

			if typ := ssaapi.GetBareType(member.GetType()); typ.GetTypeKind() == ssa.FunctionTypeKind {
				kind = "Method"
				insertText = getFuncCompletionBySSAType(label, typ)
			}
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       label,
				Description: "",
				InsertText:  insertText,
				Kind:        kind,
			})
		}
	case ssa.StringTypeKind:
		// string 内置方法
		ret = append(ret, getStringBuiltinMethodSuggestions()...)
	}
	return
}

func completionComplexStructMethodAndInstances(v *ssaapi.Value, realTyp ...ssa.Type) (ret []*ypb.SuggestionDescription) {
	var bareTyp ssa.Type
	if len(realTyp) > 0 {
		bareTyp = realTyp[0]
	} else {
		bareTyp = ssaapi.GetBareType(v.GetType())
	}
	typKind := bareTyp.GetTypeKind()
	if typKind == ssa.OrTypeKind {
		// or 类型特殊处理
		orTyp, ok := bareTyp.(*ssa.OrType)
		if !ok {
			return
		}
		for _, typ := range orTyp.GetTypes() {
			ret = append(ret, completionComplexStructMethodAndInstances(v, typ)...)
		}
		return
	}

	typStr := getGolangTypeStringBySSAType(bareTyp)
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
	return
}

func OnCompletion(prog *ssaapi.Program, word string, containPoint bool, rng *ssa.Range, v *ssaapi.Value) (ret []*ypb.SuggestionDescription) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Language completion error: %v", r)
		}
	}()
	if !containPoint {
		ret = append(ret, completionYakStandardLibrary()...)
		ret = append(ret, completionYakLanguageKeyword()...)
		ret = append(ret, completionYakLanguageBasicType()...)
		ret = append(ret, completionUserDefinedVariable(prog, rng)...)
		ret = append(ret, completionYakGlobalFunctions()...)
	} else {
		ret = append(ret, completionYakStandardLibraryChildren(v)...)
		ret = append(ret, completionYakTypeBuiltinMethod(rng, v)...)
		ret = append(ret, completionComplexStructMethodAndInstances(v)...)
	}
	if len(ret) == 0 && containPoint && v.IsUndefined() {
		// undefined means halfway through the analysis
		// so try to get the value before and complete again
		v = v.GetObject()
		if !v.IsNil() {
			return OnCompletion(prog, word, containPoint, rng, v)
		}
	}
	return ret
}

func GrpcRangeToSSARange(sourceCode string, r *ypb.Range) *ssa.Range {
	e := memedit.NewMemEditor(sourceCode)
	return ssa.NewRange(
		e,
		ssa.NewPosition(r.StartLine, r.StartColumn-1),
		ssa.NewPosition(r.EndLine, r.EndColumn-1),
	)
}

func (s *Server) YaklangLanguageSuggestion(ctx context.Context, req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, error) {
	ret := &ypb.YaklangLanguageSuggestionResponse{}

	result, err := LanguageServerAnalyzeProgram(req.GetYakScriptCode(), req.GetInspectType(), req.GetYakScriptType(), req.GetRange())
	if err != nil {
		return ret, err
	}
	prog, word, containPoint, ssaRange, v := result.Program, result.Word, result.ContainPoint, result.Range, result.Value

	if v == nil {
		return ret, nil
	}

	// todo: 处理YakScriptType，不同语言的补全、提示可能有不同
	switch req.InspectType {
	case COMPLETION:
		ret.SuggestionMessage = OnCompletion(prog, word, containPoint, ssaRange, v)
	case HOVER:
		ret.SuggestionMessage = OnHover(prog, word, containPoint, ssaRange, v)
	case SIGNATURE:
		ret.SuggestionMessage = OnSignature(prog, word, containPoint, ssaRange, v)
	}
	return ret, nil
}
