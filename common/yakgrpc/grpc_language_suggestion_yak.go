package yakgrpc

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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

	externValueSuggestionsMap = utils.NewTTLCache[[]*ypb.SuggestionDescription](15 * time.Minute)

	standardLibrarySuggestions = make([]*ypb.SuggestionDescription, 0)
	yakKeywordSuggestions      = make([]*ypb.SuggestionDescription, 0)
	yakTypeSuggestions         = make([]*ypb.SuggestionDescription, 0)
	progCacheMap               = utils.NewTTLCache[*ssaapi.Program](0)

	CompletionKindField    = "Field"
	CompletionKindKeyword  = "Keyword"
	CompletionKindConstant = "Constant"
	CompletionKindVariable = "Variable"
	CompletionKindFunction = "Function"
	CompletionKindMethod   = "Method"
	CompletionKindClass    = "Class"
	CompletionKindModule   = "Module"
	CompletionKindSnippet  = "Snippet"
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
				Kind:        CompletionKindKeyword,
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
				Kind:        CompletionKindClass,
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
				Kind:        CompletionKindMethod,
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
				Kind:        CompletionKindMethod,
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
				Kind:        CompletionKindMethod,
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
				Kind:              CompletionKindMethod,
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
		standardLibrarySuggestions = make([]*ypb.SuggestionDescription, 0, len(doc.GetDefaultDocumentHelper().Libs))
		for libName := range doc.GetDefaultDocumentHelper().Libs {
			standardLibrarySuggestions = append(standardLibrarySuggestions, &ypb.SuggestionDescription{
				Label:       libName,
				InsertText:  libName,
				Description: "Standard Library",
				Kind:        CompletionKindModule,
			})
		}
	}

	return standardLibrarySuggestions
}

func getSSAFunctionVscodeSnippets(funcName string, funTyp *ssa.FunctionType) string {
	snippet := funcName
	parameter := funTyp.Parameter
	if funTyp.IsMethod {
		if len(parameter) > 0 {
			parameter = parameter[1:]
		}
	}
	lenOfParams := len(parameter)
	snippet += "("
	snippet += strings.Join(
		lo.Map(parameter, func(typ ssa.Type, i int) string {
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
	funcDecls := doc.GetDefaultDocumentHelper().Functions
	if libName != "" {
		lib, ok := doc.GetDefaultDocumentHelper().Libs[libName]
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
	instances := doc.GetDefaultDocumentHelper().Instances

	if libName != "" {
		lib, ok := doc.GetDefaultDocumentHelper().Libs[libName]
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

func _getGolangTypeStringBySSAType(typ ssa.Type) string {
	typStr := typ.PkgPathString()
	if typStr == "" {
		typStr = typ.String()
	}
	return _getGolangTypeStringByTypeStr(typStr)
}

func _prettyGolangTypeStringBySSAType(typ ssa.Type) string {
	typStr := _getGolangTypeStringBySSAType(typ)
	if strings.Contains(typStr, "/") {
		splited := strings.Split(typStr, "/")
		typStr = splited[len(splited)-1]
	}
	return typStr
}

func _getGolangTypeStringByTypeStr(typStr string) string {
	switch typStr {
	case "boolean":
		return "bool"
	case "bytes":
		return "[]byte"
	}
	return typStr
}

func _shouldExport(key string) bool {
	return (key[0] >= 'A' && key[0] <= 'Z')
}

func _markdownWrapper(desc string) string {
	return yakdoc.ShrinkTypeVerboseName(fmt.Sprintf("```go\n%s\n```", desc))
}

func getFuncDeclDesc(v *ssaapi.Value, funcDecl *yakdoc.FuncDecl) string {
	label, doc := getFuncDeclLabel(v, funcDecl), funcDecl.Document
	return _getFuncDescFromLabelAndDoc(label, doc)
}

func getFuncDeclLabel(v *ssaapi.Value, funcDecl *yakdoc.FuncDecl) string {
	var (
		funcName     = funcDecl.MethodName
		decl         = funcDecl.Decl
		desc, prefix string
	)

	if strings.HasPrefix(decl, "func(") {
		// fix decl
		decl = strings.Replace(decl, "func(", funcName+"(", 1)
	}

	if v != nil {
		bareV := v.GetSSAInst()
		fValue, isFunction := ssa.ToFunction(bareV)
		typ := ssaapi.GetBareType(v.GetType())
		funcTyp, ok := ssa.ToFunctionType(typ)
		if ok {
			isMethod := funcTyp.IsMethod
			if isMethod && len(funcTyp.Parameter) > 0 {
				prefix = fmt.Sprintf("(%s) ", funcTyp.Parameter[0])
			}

			if isFunction && fValue.IsGeneric() {
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

				desc = fmt.Sprintf("%s%s(%s) %s", prefix, funcName, paramsStr, returnsStr)
			}
		}
	}

	if desc == "" {
		desc = fmt.Sprintf("func %s%s", prefix, decl)
	}

	return desc
}

func getConstInstanceDesc(instance *yakdoc.LibInstance) string {
	desc := _markdownWrapper(fmt.Sprintf("const %s = %s", instance.InstanceName, instance.ValueStr))
	return desc
}

func _getFuncTypeDesc(funcTyp *ssa.FunctionType, funcName string) string {
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
	return desc
}

func _getFuncDescByDecls(funcDecls map[string]*yakdoc.FuncDecl, callback func(decl *yakdoc.FuncDecl) string) string {
	desc := ""
	methodNames := utils.GetSortedMapKeys(funcDecls)

	for _, methodName := range methodNames {
		desc += callback(funcDecls[methodName])
	}

	return desc
}

func _getFuncDescByTypeStr(typStr string, typName string, isStruct, tab bool) string {
	lib, ok := doc.GetDefaultDocumentHelper().StructMethods[typStr]
	if !ok {
		return ""
	}

	return _getFuncDescByDecls(lib.Functions, func(decl *yakdoc.FuncDecl) string {
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

func _getBuiltinFuncDeclAndDoc(name string, bareTyp ssa.Type) (desc string, doc string) {
	var m map[string]*ypb.SuggestionDescription
	if utils.IsNil(bareTyp) {
		return
	}
	switch bareTyp.GetTypeKind() {
	case ssa.SliceTypeKind:
		// []byte / [] 内置方法
		rTyp, ok := ssa.ToObjectType(bareTyp)
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

func getFuncLabelAndDocBySSAValue(name string, v *ssaapi.Value) (label string, document string) {
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
		parentTypStr = _getGolangTypeStringBySSAType(parentBareTyp)
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
				return getFuncDeclLabel(v, funcDecl), funcDecl.Document
			}
		}
	}

	// 结构体 / 接口方法
	lib, ok := doc.GetDefaultDocumentHelper().StructMethods[parentTypStr]
	if ok {
		funcDecl, ok := lib.Functions[lastName]
		if ok {
			return getFuncDeclLabel(v, funcDecl), funcDecl.Document
		}
	}

	// 类型内置方法, 方法签名现在用 SSA Value 获取
	funcObjectType := v.GetFunctionObjectType()
	_, document = _getBuiltinFuncDeclAndDoc(lastName, funcObjectType)

	// 用户自定义函数
	if funcTyp != nil {
		label = _getFuncTypeDesc(funcTyp, lastName)
		return
	}

	return
}

func getExternLibDesc(name string) string {
	// 标准库
	lib, ok := doc.GetDefaultDocumentHelper().Libs[name]
	if !ok {
		// break
		return ""
	}

	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("package %s\n\n", name))
	instanceKeys := utils.GetSortedMapKeys(lib.Instances)
	for _, key := range instanceKeys {
		instance := lib.Instances[key]
		builder.WriteString(fmt.Sprintf("const %s %s = %s\n", instance.InstanceName, _getGolangTypeStringByTypeStr(instance.Type), instance.ValueStr))
	}
	builder.WriteRune('\n')
	builder.WriteString(_getFuncDescByDecls(lib.Functions, func(decl *yakdoc.FuncDecl) string {
		return fmt.Sprintf("func %s\n", decl.Decl)
	}))
	return _markdownWrapper(builder.String())
}

func _getFuncDescFromLabelAndDoc(desc, doc string) string {
	if doc == "" {
		return _markdownWrapper(desc)
	}
	return _markdownWrapper(desc) + "\n\n" + doc
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
	typStr := _getGolangTypeStringBySSAType(bareTyp)
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
					// doc := funcDecl.Document
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
		label, doc := getFuncLabelAndDocBySSAValue(name, v)
		desc = _getFuncDescFromLabelAndDoc(label, doc)
	case ssa.StructTypeKind:
		rTyp, ok := bareTyp.(*ssa.ObjectType)
		if !ok {
			break
		}
		if rTyp.Combination {
			desc = _markdownWrapper(fmt.Sprintf("%s (%s)", name, typStr))
			break
		}
		desc = fmt.Sprintf("type %s struct {\n", shortTypName)
		for _, key := range rTyp.Keys {
			// 过滤掉非导出字段
			if !_shouldExport(key.String()) {
				continue
			}
			fieldType := rTyp.GetField(key)
			desc += fmt.Sprintf("    %-20s %s\n", key, _prettyGolangTypeStringBySSAType(fieldType))
		}
		desc += "}"
		methodDescriptions := _getFuncDescByTypeStr(typStr, shortTypName, true, false)
		if methodDescriptions != "" {
			desc += "\n\n"
			desc += methodDescriptions
		}
		desc = _markdownWrapper(desc)
	case ssa.InterfaceTypeKind:
		desc = fmt.Sprintf("type %s interface {\n", shortTypName)
		methodDescriptions := _getFuncDescByTypeStr(typStr, shortTypName, false, true)
		desc += methodDescriptions
		desc += "}"
		desc = _markdownWrapper(desc)
	}

	// 结构体成员
	if desc == "" {
		parentV := v.GetObject()
		if parentV != nil {
			parentBareTyp := ssaapi.GetBareType(parentV.GetType())
			parentTypStr := _getGolangTypeStringBySSAType(parentBareTyp)
			lib, ok := doc.GetDefaultDocumentHelper().StructMethods[parentTypStr]
			if ok {
				instance, ok := lib.Instances[lastName]
				if ok {
					desc = _markdownWrapper(
						fmt.Sprintf("field %s %s",
							instance.InstanceName,
							_getGolangTypeStringByTypeStr(instance.Type)))
				}
			}
		}
	}

	if desc == "" {
		desc = _markdownWrapper(fmt.Sprintf("type %s %s", lastName, typStr))
	}
	return desc
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

func trimSourceCode(sourceCode string) (code string, containPoint bool, pointSuffix bool) {
	containPoint = strings.Contains(sourceCode, ".")
	pointSuffix = strings.HasSuffix(sourceCode, ".")
	if pointSuffix {
		sourceCode = sourceCode[:len(sourceCode)-1]
	}
	return strings.TrimSpace(sourceCode), containPoint, pointSuffix
}

func OnHover(prog *ssaapi.Program, word string, containPoint bool, rng *memedit.Range, v *ssaapi.Value) (ret []*ypb.SuggestionDescription) {
	ret = append(ret, &ypb.SuggestionDescription{
		Label: getDescFromSSAValue(word, containPoint, prog, v),
	})

	return ret
}

func OnSignature(prog *ssaapi.Program, word string, containPoint bool, rng *memedit.Range, v *ssaapi.Value) (ret []*ypb.SuggestionDescription) {
	ret = make([]*ypb.SuggestionDescription, 0)

	label, doc := getFuncLabelAndDocBySSAValue(word, v)
	if label != "" {
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       label,
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

func completionUserDefinedVariable(prog *ssaapi.Program, rng *memedit.Range, filterMap map[string]struct{}) (ret []*ypb.SuggestionDescription) {
	if prog == nil || prog.Program == nil {
		return
	}

	ret = make([]*ypb.SuggestionDescription, 0)
	// 自定义变量补全
	// 需要反转，因为是按 offset 顺序排列的
	for _, item := range lo.Reverse(prog.GetAllOffsetItemsBefore(rng.GetEndOffset())) {
		variable := item.GetVariable()
		varName := variable.GetName()
		if _, ok := filterMap[varName]; ok {
			continue
		}
		filterMap[varName] = struct{}{}
		bareValue := item.GetValue()
		v, err := prog.NewValue(bareValue)
		if err != nil {
			continue
		}
		bareTyp := ssaapi.GetBareType(v.GetType())
		typStr := _getGolangTypeStringBySSAType(bareTyp)

		// 不应该再补全标准库函数和标准库
		if _, ok := doc.GetDefaultDocumentHelper().Functions[varName]; ok {
			continue
		}
		if _, ok := doc.GetDefaultDocumentHelper().Libs[varName]; ok {
			continue
		}
		// 不应该再补全包含.或#的符号
		if strings.Contains(varName, ".") || strings.Contains(varName, "#") {
			continue
		}

		insertText := varName
		vKind := CompletionKindVariable
		if !v.IsNil() && v.IsFunction() {
			vKind = CompletionKindFunction
			funcTyp, _ := ssa.ToFunctionType(bareValue.GetType())
			insertText = getSSAFunctionVscodeSnippets(varName, funcTyp)
		}
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       varName,
			Description: typStr,
			InsertText:  insertText,
			Kind:        vKind,
		})
	}
	return
}

func completionExternValues(prog *ssaapi.Program, filterMap map[string]struct{}) (ret []*ypb.SuggestionDescription) {
	functions := doc.GetDefaultDocumentHelper().Functions
	ret = make([]*ypb.SuggestionDescription, 0, len(functions))

	for name, value := range prog.Program.ExternInstance {
		if strings.HasPrefix(name, "$") {
			continue
		}
		filterMap[name] = struct{}{}
		if funcDecl, ok := functions[name]; ok {
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       name,
				Description: funcDecl.Document,
				InsertText:  funcDecl.VSCodeSnippets,
				Kind:        CompletionKindFunction,
			})
		} else {
			bareValue := prog.Program.BuildValueFromAny(nil, name, value)
			v, err := prog.NewValue(bareValue)
			if err != nil {
				continue
			}

			insertText := name
			desc := ""
			vKind := CompletionKindVariable

			if !v.IsNil() && v.IsFunction() {
				vKind = CompletionKindFunction
				funcTyp, _ := ssa.ToFunctionType(bareValue.GetType())
				insertText = getSSAFunctionVscodeSnippets(name, funcTyp)
			} else {
				bareTyp := ssaapi.GetBareType(v.GetType())
				desc = _getGolangTypeStringBySSAType(bareTyp)
			}
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       name,
				Description: desc,
				InsertText:  insertText,
				Kind:        vKind,
			})
		}
	}
	return
}

func completionYakStandardLibraryChildren(v *ssaapi.Value, word string) (ret []*ypb.SuggestionDescription) {
	libName := word
	if v.IsExternLib() {
		libName = v.GetName()
	}
	lib, ok := doc.GetDefaultDocumentHelper().Libs[libName]
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
				Kind:        CompletionKindFunction,
			})
		}
	}
	if len(lib.Instances) > 0 {
		for _, instance := range lib.Instances {
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       instance.InstanceName,
				Description: "",
				InsertText:  instance.InstanceName,
				Kind:        CompletionKindConstant,
			})
		}
	}
	return
}

func completionYakTypeBuiltinMethod(rng *memedit.Range, v *ssaapi.Value, realTyp ...ssa.Type) (ret []*ypb.SuggestionDescription) {
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

			vKind := CompletionKindField
			insertText := ""
			label := key.String()
			if kind := key.GetTypeKind(); kind == ssa.StringTypeKind || kind == ssa.BytesTypeKind {
				label, _ = strconv.Unquote(label)
			}
			insertText = label

			if typ := ssaapi.GetBareType(member.GetType()); typ.GetTypeKind() == ssa.FunctionTypeKind {
				vKind = CompletionKindMethod
				insertText = getFuncCompletionBySSAType(label, typ)
			}
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       label,
				Description: "",
				InsertText:  insertText,
				Kind:        vKind,
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

	typStr := _getGolangTypeStringBySSAType(bareTyp)
	// 接口方法，结构体成员与方法，定义类型方法
	lib, ok := doc.GetDefaultDocumentHelper().StructMethods[typStr]
	if ok {
		for _, instance := range lib.Instances {
			// 过滤掉非导出字段
			if !_shouldExport(instance.InstanceName) {
				continue
			}
			keyStr := instance.InstanceName
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       keyStr,
				Description: "",
				InsertText:  keyStr,
				Kind:        CompletionKindField,
			})
		}

		for methodName, funcDecl := range lib.Functions {
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       methodName,
				Description: funcDecl.Document,
				InsertText:  funcDecl.VSCodeSnippets,
				Kind:        CompletionKindMethod,
			})
		}
		return

	} else if objType, ok := ssa.ToObjectType(bareTyp); (typKind == ssa.ClassBluePrintTypeKind || typKind == ssa.ObjectTypeKind) && ok {
		for _, key := range objType.Keys {
			keyStr := key.String()
			// 过滤掉非导出字段
			fieldType := objType.GetField(key)
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       keyStr,
				Description: fieldType.String(),
				InsertText:  keyStr,
				Kind:        CompletionKindField,
			})
		}
		for methodName, method := range objType.GetMethod() {
			funcTyp, _ := ssa.ToFunctionType(method.GetType())
			insertText := getSSAFunctionVscodeSnippets(methodName, funcTyp)
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       methodName,
				Description: "",
				InsertText:  insertText,
				Kind:        CompletionKindMethod,
			})
		}
	}

	return
}

func fixCompletionFunctionParams(suggestions []*ypb.SuggestionDescription, v *ssaapi.Value) []*ypb.SuggestionDescription {
	// fix completion, for function params that are function type, we should complete function name instead of function signature
	// e.g. callable(app) -> callable(append), not callable(append(a, vals...))
	users := v.GetUsers()
	if len(users) == 0 {
		return suggestions
	}
	sort.SliceStable(users, func(i, j int) bool {
		return users[i].GetRange().GetEndOffset() < users[j].GetRange().GetEndOffset()
	})
	lastUser := users[len(users)-1]
	if !lastUser.IsCall() {
		return suggestions
	}
	call, ok := ssa.ToCall(lastUser.GetSSAInst())
	if !ok {
		return suggestions
	}
	method, ok := call.GetValueById(call.Method)
	if !ok || method == nil {
		return suggestions
	}
	funcTyp, ok := ssa.ToFunctionType(method.GetType())
	if !ok {
		return suggestions
	}
	// find index of call.Args
	index := -1
	for i, arg := range call.Args {
		if arg == v.GetId() {
			index = i
		}
	}
	if index == -1 {
		return suggestions
	}
	if len(funcTyp.Parameter) <= index {
		return suggestions
	}
	paramTyp := funcTyp.Parameter[index]
	if paramTyp.GetTypeKind() != ssa.FunctionTypeKind {
		return suggestions
	}
	if ssa.TypeCompare(paramTyp, ssaapi.GetBareType(v.GetType())) {
		for _, r := range suggestions {
			if r.Kind != CompletionKindFunction && r.Kind != CompletionKindMethod {
				continue
			}
			if index := strings.Index(r.InsertText, "("); index != -1 {
				r.InsertText = r.InsertText[:index]
			}
		}
	}
	return suggestions
}

func fixCompletionBeforeParen(suggestions []*ypb.SuggestionDescription, prog *ssaapi.Program, rng *memedit.Range, v *ssaapi.Value) []*ypb.SuggestionDescription {
	// fix completion, for text before paren, we should complete function name instead of function signature
	// e.g. callable(app()) -> callable(append()), not callable(append(a, vals...)())
	editor, ok := prog.Program.GetEditor("")
	if !ok {
		return suggestions
	}
	text := editor.GetTextFromOffset(rng.GetEndOffset(), rng.GetEndOffset()+1)
	if text != "(" {
		return suggestions
	}
	for _, r := range suggestions {
		if r.Kind != CompletionKindFunction && r.Kind != CompletionKindMethod {
			continue
		}
		if index := strings.Index(r.InsertText, "("); index != -1 {
			r.InsertText = r.InsertText[:index]
		}
	}
	return suggestions
}

// getExpectedParamTypeAtArg 反查「当前正在补全的值 v」所处的函数调用实参位置，
// 返回该实参位置期望的形参类型。若该形参是变长参数(最后一个)，会解包成元素类型。
// 典型场景：poc.saveHandler(f) 里补全 f 时，反查出期望类型 func(*lowhttp.LowhttpResponse)。
// 关键词: 回调函数补全, 实参形参类型推断, 变长参数解包
func getExpectedParamTypeAtArg(v *ssaapi.Value) (ssa.Type, bool) {
	if v == nil || v.IsNil() {
		return nil, false
	}
	users := v.GetUsers()
	if len(users) == 0 {
		return nil, false
	}
	// 取 EndOffset 最大的 user 作为最近的一次使用(最近的调用)
	sort.SliceStable(users, func(i, j int) bool {
		return users[i].GetRange().GetEndOffset() < users[j].GetRange().GetEndOffset()
	})
	lastUser := users[len(users)-1]
	if !lastUser.IsCall() {
		return nil, false
	}
	call, ok := ssa.ToCall(lastUser.GetSSAInst())
	if !ok {
		return nil, false
	}
	method, ok := call.GetValueById(call.Method)
	if !ok || method == nil {
		return nil, false
	}
	funcTyp, ok := ssa.ToFunctionType(method.GetType())
	if !ok || funcTyp == nil {
		return nil, false
	}
	// 找到 v 作为实参出现的下标
	index := -1
	for i, arg := range call.Args {
		if arg == v.GetId() {
			index = i
		}
	}
	if index == -1 {
		// v 本身就是被调用的函数(光标紧贴左括号、实参尚为空)，按第一个形参处理
		if call.Method == v.GetId() {
			index = 0
		} else {
			return nil, false
		}
	}
	return getFunctionParamTypeByIndex(funcTyp, index)
}

// getFunctionParamTypeByIndex 依据形参下标取形参类型；最后一个变长形参会解包成元素类型。
func getFunctionParamTypeByIndex(funcTyp *ssa.FunctionType, index int) (ssa.Type, bool) {
	n := len(funcTyp.Parameter)
	if n == 0 || index < 0 {
		return nil, false
	}
	last := n - 1
	if index > last {
		// 超出显式形参数量的实参落在变长形参上
		if funcTyp.IsVariadic {
			index = last
		} else {
			return nil, false
		}
	}
	paramTyp := funcTyp.Parameter[index]
	// 变长最后一个形参在 SSA 里以 slice 存储，解包成元素类型
	if funcTyp.IsVariadic && index == last {
		if obj, ok := ssa.ToObjectType(paramTyp); ok && obj.Kind == ssa.SliceTypeKind && obj.FieldType != nil {
			return obj.FieldType, true
		}
	}
	return paramTyp, true
}

// lowerCamelInitialism 把类型名转成 lowerCamel 变量名，友好处理首字母缩写：
// HTTPFlow -> httpFlow, URL -> url, ID -> id, ResponseWriter -> responseWriter。
func lowerCamelInitialism(name string) string {
	if name == "" {
		return name
	}
	// 统计开头连续的大写字母数量
	run := 0
	for _, r := range name {
		if r >= 'A' && r <= 'Z' {
			run++
		} else {
			break
		}
	}
	switch {
	case run == 0:
		return name
	case run == len(name):
		// 全大写(如 URL / ID)整体转小写
		return strings.ToLower(name)
	case run == 1:
		return strings.ToLower(name[:1]) + name[1:]
	default:
		// 开头是缩写(如 HTTPFlow)：最后一个大写字母作为下一个单词的开头
		return strings.ToLower(name[:run-1]) + name[run-1:]
	}
}

// callbackParamFriendlyName 是「回调形参短类型名 -> 更贴近文档习惯的占位名」注册表。
// 短类型名 = 去掉包路径与 */[]& 前缀后的类型名(大小写敏感，与 Go 类型一致)。
// 这样 poc.saveHandler 的回调补出 func(rsp) 而不是 func(lowhttpResponse)。
// 关键词: 回调形参名映射, 友好占位名, rsp, flow, req
var callbackParamFriendlyName = map[string]string{
	"LowhttpResponse": "rsp",
	"Response":        "rsp",
	"HTTPFlow":        "flow",
	"Request":         "req",
	"ResponseWriter":  "w",
	"SynScanResult":   "result",
	"BruteItem":       "item",
	"BruteItemResult": "result",
	"tcpConnection":   "conn",
	"udpConnection":   "conn",
	"Conn":            "conn",
	"Client":          "client",
	"Packet":          "packet",
	"HTTPFlowT":       "flow",
}

// friendlyCallbackParamName 依据类型串返回注册表里的友好形参名，未命中返回空串。
func friendlyCallbackParamName(typStr string) string {
	t := strings.TrimSpace(typStr)
	// 常见基础类型的惯用命名(同时规避与关键字/类型名冲突)
	switch t {
	case "[]byte", "[]uint8", "bytes":
		return "data"
	case "string":
		return "s"
	case "bool", "boolean":
		return "ok"
	case "error":
		return "err"
	}
	isSlice := strings.HasPrefix(t, "[]")
	core := strings.TrimLeft(t, "*[]&")
	if i := strings.LastIndex(core, "."); i != -1 {
		core = core[i+1:]
	}
	if name, ok := callbackParamFriendlyName[core]; ok {
		if isSlice {
			// 切片形参用复数，如 reqs / flows
			return name + "s"
		}
		return name
	}
	return ""
}

// callbackParamPlaceholderName 依据形参类型推导一个可读的占位形参名，冲突时追加序号。
// 优先使用友好名注册表(rsp/flow/req...)，未命中再从类型名推导。
// 例如 *lowhttp.LowhttpResponse -> rsp; *schema.HTTPFlow -> flow; 未知类型 -> 类型名 lowerCamel。
func callbackParamPlaceholderName(typStr string, index int, seen map[string]int) string {
	name := friendlyCallbackParamName(typStr)
	if name == "" {
		// 回退：从类型名推导
		name = typStr
		// 去掉指针/切片等前缀符号
		name = strings.TrimLeft(name, "*[]&")
		// 去掉包路径，只保留最后一段类型名
		if i := strings.LastIndex(name, "."); i != -1 {
			name = name[i+1:]
		}
		// 只保留合法标识符字符(去掉泛型尖括号等)
		name = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				return r
			}
			return -1
		}, name)
		if name == "" {
			name = fmt.Sprintf("p%d", index+1)
		} else {
			name = lowerCamelInitialism(name)
		}
	}
	// 与 yak 关键字或基础类型名冲突则加后缀，避免生成非法/易混淆代码
	if utils.StringArrayContains(yakKeywords, name) || utils.StringArrayContains(yakTypes, name) {
		name += "Param"
	}
	// 同名去重
	if c, ok := seen[name]; ok {
		seen[name] = c + 1
		return fmt.Sprintf("%s%d", name, c+1)
	}
	seen[name] = 1
	return name
}

// buildCallbackFunctionSuggestions 根据回调函数类型生成两种可 tab 展开的字面量补全：
// func 声明式 `func(params) { }` 与箭头式 `(params) => { }`。
// 关键词: 回调函数字面量补全, func 声明, 箭头函数
func buildCallbackFunctionSuggestions(cbTyp *ssa.FunctionType) []*ypb.SuggestionDescription {
	params := cbTyp.Parameter
	n := len(params)
	seen := make(map[string]int)
	labelParams := make([]string, 0, n)
	snippetParams := make([]string, 0, n)
	for i, p := range params {
		typStr := p.String()
		if cbTyp.IsVariadic && i == n-1 {
			// 回调本身也可能是变长，兜底解包元素类型用于命名
			if obj, ok := ssa.ToObjectType(p); ok && obj.Kind == ssa.SliceTypeKind && obj.FieldType != nil {
				typStr = obj.FieldType.String()
			}
		}
		name := callbackParamPlaceholderName(typStr, i, seen)
		labelParams = append(labelParams, name)
		snippetParams = append(snippetParams, fmt.Sprintf("${%d:%s}", i+1, name))
	}
	labelArgs := strings.Join(labelParams, ", ")
	snippetArgs := strings.Join(snippetParams, ", ")

	desc := "callback " + _getFuncTypeDesc(cbTyp, "")
	// 无返回值时 ReturnType 会渲染成 " null"，去掉以免文档难看
	desc = strings.TrimSuffix(desc, " null")
	funcLabel := fmt.Sprintf("func(%s) {}", labelArgs)
	arrowLabel := fmt.Sprintf("(%s) => {}", labelArgs)

	// 回调有返回值时，函数体预置 return 骨架，减少用户后续输入
	body := "\t$0\n"
	if cbTyp.ReturnType != nil && cbTyp.ReturnType.GetTypeKind() != ssa.NullTypeKind {
		body = "\treturn $0\n"
	}

	return []*ypb.SuggestionDescription{
		{
			Label:             funcLabel,
			InsertText:        fmt.Sprintf("func(%s) {\n%s}", snippetArgs, body),
			Description:       desc,
			DefinitionVerbose: funcLabel,
			Kind:              CompletionKindSnippet,
		},
		{
			Label:             arrowLabel,
			InsertText:        fmt.Sprintf("(%s) => {\n%s}", snippetArgs, body),
			Description:       desc,
			DefinitionVerbose: arrowLabel,
			Kind:              CompletionKindSnippet,
		},
	}
}

// completionCallbackFunctionLiteral 当当前正在补全的实参期望一个函数类型时，
// 生成回调函数字面量补全(func 声明式 + 箭头式)，便于用户直接 tab 展开成回调骨架。
// 例如 poc.saveHandler(f) / poc.afterSaveHandler(f) 等以函数为参数的库调用。
// 关键词: poc.saveHandler 自动补全, 回调函数参数, func 字面量补全
func completionCallbackFunctionLiteral(v *ssaapi.Value) []*ypb.SuggestionDescription {
	paramTyp, ok := getExpectedParamTypeAtArg(v)
	if !ok {
		return nil
	}
	cbTyp, ok := ssa.ToFunctionType(paramTyp)
	if !ok || cbTyp == nil {
		return nil
	}
	return buildCallbackFunctionSuggestions(cbTyp)
}

func OnCompletion(
	prog *ssaapi.Program, word string, containPoint bool, pointSuffix bool,
	rng *memedit.Range, scriptType string, v *ssaapi.Value,
) (ret []*ypb.SuggestionDescription) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Language completion error: %v", r)
		}
		ret = fixCompletionFunctionParams(ret, v)
		ret = fixCompletionBeforeParen(ret, prog, rng, v)
	}()
	if !containPoint {
		// 当前实参期望函数类型时，优先给出回调函数字面量补全(func 声明式/箭头式)
		ret = append(ret, completionCallbackFunctionLiteral(v)...)
		ret = append(ret, completionYakStandardLibrary()...)
		ret = append(ret, completionYakLanguageKeyword()...)
		ret = append(ret, completionYakLanguageBasicType()...)
		filterMap := make(map[string]struct{})
		ret = append(ret, completionExternValues(prog, filterMap)...)
		ret = append(ret, completionUserDefinedVariable(prog, rng, filterMap)...)
	} else {
		ret = append(ret, completionYakStandardLibraryChildren(v, word)...)
		ret = append(ret, completionYakTypeBuiltinMethod(rng, v)...)
		ret = append(ret, completionComplexStructMethodAndInstances(v)...)
	}
	if len(ret) == 0 && containPoint && !pointSuffix && v.IsUndefined() {
		/*
			when member completion item is empty  use object completion , for `a.bb` completion all a member
			but when pointSuffix=true: `a.b.`, b is the object, not `a`,
		*/
		obj := v.GetObject()
		if obj.IsNil() {
			return ret
		}

		undefined, ok := ssa.ToUndefined(v.GetSSAInst())
		if !ok {
			return ret
		}

		// should check if key is member
		if undefined.Kind != ssa.UndefinedMemberValid && undefined.Kind != ssa.UndefinedMemberInValid {
			return ret
		}

		// undefined means halfway through the analysis
		// so try to get the value before and complete again
		return OnCompletion(prog, word, containPoint, pointSuffix, rng, scriptType, obj)
	}
	return ret
}

func (s *Server) YaklangLanguageSuggestion(ctx context.Context, req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, error) {
	// check syntaxflow
	if resp, match := SyntaxFlowServer(req); match {
		return applyExampleFenceToResponse(resp), nil
	}

	if resp, match := FuzztagServer(req); match {
		return applyExampleFenceToResponse(resp), nil
	}

	scriptType := req.GetYakScriptType()
	ret := &ypb.YaklangLanguageSuggestionResponse{}
	switch scriptType {
	case "yak", "mitm", "port-scan", "codec":
		// do nothing
	default:
		// unsupported script type
		return ret, utils.Errorf("unsupported script type: %s", scriptType)
	}

	result, err := LanguageServerAnalyzeProgram(req)
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
		ret.SuggestionMessage = OnCompletion(prog, word, containPoint, result.PointSuffix, ssaRange, scriptType, v)
	case HOVER:
		ret.SuggestionMessage = OnHover(prog, word, containPoint, ssaRange, v)
	case SIGNATURE:
		ret.SuggestionMessage = OnSignature(prog, word, containPoint, ssaRange, v)
	}
	// 发送给前端展示前的最后一刻：把文档里的 <|EXAMPLE...|> 标记渲染成代码围栏
	return applyExampleFenceToResponse(ret), nil
}