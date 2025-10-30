package yakgrpc

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

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

var _fuzztagSuggestions []*ypb.SuggestionDescription
var _fuzztagDescMap = make(map[string]string)
var _fuzztagSuggestionsOnce sync.Once
var tagDescFormatString = "**%s**\n\n%s\n\n**Example:**\n\n```http\n%s\n```"

func getFuzztagSuggestion(tagName string, labelFormatString string, tagDesc *mutate.FuzzTagDescription) (string, *ypb.SuggestionDescription) {
	tagLabel := fmt.Sprintf(labelFormatString, tagName, tagDesc.TagNameVerbose)
	return tagLabel, &ypb.SuggestionDescription{
		Label:       tagLabel,
		Description: tagDesc.Description,
		InsertText:  fmt.Sprintf(`%s($1)}}`, tagName),
		Kind:        CompletionKindFunction,
	}
}

func _getAllFuzztagSuggestionInfo() ([]*ypb.SuggestionDescription, map[string]string) {
	_fuzztagSuggestionsOnce.Do(func() {
		allTag := append(mutate.GetAllFuzztags(), append(mutate.FileTag(), mutate.CodecTag()...)...)
		allTag = append(allTag, mutate.HotPatchFuzztag(func(s string, f func(string)) error { return nil }), mutate.HotPatchDynFuzztag(func(s string, f func(string)) error { return nil }))
		allTag = append(allTag, &mutate.FuzzTagDescription{TagName: "request", Description: "原始请求", TagNameVerbose: "request", Examples: []string{"{{request}}"}})
		tagLabelFormatString := fmt.Sprintf("%%-%ds[%%s]", mutate.GetFuzztagMaxLength(allTag)+4)
		for _, tag := range allTag {
			// tag name suggestion
			tagLabel, tagSuggestion := getFuzztagSuggestion(tag.TagName, tagLabelFormatString, tag)
			_fuzztagSuggestions = append(_fuzztagSuggestions, tagSuggestion)
			_fuzztagDescMap[tag.TagName] = fmt.Sprintf(tagDescFormatString, tagLabel, tag.Description, strings.Join(tag.Examples, "\n"))

			for _, alias := range tag.Alias { // alias suggesion
				aliasLabel, aliasTagSuggestion := getFuzztagSuggestion(alias, tagLabelFormatString, tag)
				_fuzztagSuggestions = append(_fuzztagSuggestions, aliasTagSuggestion)
				_fuzztagDescMap[alias] = fmt.Sprintf(tagDescFormatString, aliasLabel, tag.Description, strings.Join(lo.Map(tag.Examples, func(item string, index int) string {
					return strings.Replace(item, tag.TagName, alias, 1)
				}), "\n"))
			}
		}
	})
	return _fuzztagSuggestions, _fuzztagDescMap
}

func getCodecPluginList() []*ypb.SuggestionDescription {
	var ret []*ypb.SuggestionDescription
	for _, codecScript := range yakit.QueryYakScriptByType(consts.GetGormProfileDatabase(), "codec") {
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       codecScript.ScriptName,
			Description: codecScript.Help,
			InsertText:  codecScript.ScriptName,
		})
	}
	return ret
}

func getPayloadGroup() []*ypb.SuggestionDescription {
	var ret []*ypb.SuggestionDescription
	allPayloadGroup, _ := yakit.GetAllPayloadGroupName(consts.GetGormProfileDatabase())
	for _, groupName := range allPayloadGroup {
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       groupName,
			Description: "",
			InsertText:  groupName,
		})
	}
	return ret
}

func FuzztagServer(req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, bool) {
	if req.GetYakScriptType() != "fuzztag" {
		return nil, false
	}
	ret := &ypb.YaklangLanguageSuggestionResponse{}
	switch req.GetInspectType() {
	case COMPLETION:
		// ret
		ret.SuggestionMessage = append(ret.SuggestionMessage, fuzztagCompletion(req.GetRange().GetCode(), "")...)
	case HOVER:
		// ret
		ret.SuggestionMessage = append(ret.SuggestionMessage, fuzztagHover(req.GetRange().Code, "")...)
	}
	return ret, true
}

func fuzztagHover(fuzztagCode string, hotPatchCode string) []*ypb.SuggestionDescription {
	var suggestions []*ypb.SuggestionDescription
	_, descMap := _getAllFuzztagSuggestionInfo()
	desc, ok := descMap[fuzztagCode]
	if ok {
		suggestions = append(suggestions, &ypb.SuggestionDescription{
			Label: desc,
		})
	}
	return suggestions
}

var hotPatchBlacklist = []string{"afterRequest", "beforeRequest", "mirrorHTTPFlow"}

func fuzztagCompletion(fuzztagCode string, hotPatchCode string) []*ypb.SuggestionDescription {
	var suggestions []*ypb.SuggestionDescription
	var hotPatchSuggestions []*ypb.SuggestionDescription

	if hotPatchCode != "" {
		prog, err := static_analyzer.SSAParse(hotPatchCode, string(plugin_type.PluginTypeYak), ssaapi.WithIgnoreSyntaxError(true))
		if err == nil {
			mainFunc, ok := prog.Program.Funcs.Get(string(ssa.MainFunctionName))
			if ok {
				for _, childFunc := range mainFunc.ChildFuncs {
					childFunc, ok := mainFunc.GetValueById(childFunc)
					if !ok || childFunc == nil {
						continue
					}
					if utils.StringArrayContains(hotPatchBlacklist, childFunc.GetName()) {
						continue
					}
					funcTyp, _ := ssa.ToFunctionType(childFunc.GetType())
					hotPatchSuggestions = append(hotPatchSuggestions, &ypb.SuggestionDescription{
						Label:       childFunc.GetName(),
						InsertText:  childFunc.GetName(),
						Kind:        CompletionKindFunction,
						Description: funcTyp.String(),
					})
				}
			}
		}
	}

	if strings.HasPrefix(fuzztagCode, "{{") {
		if strings.HasPrefix(fuzztagCode, "{{codec(") {
			return getCodecPluginList()
		}
		if strings.HasPrefix(fuzztagCode, "{{payload(") {
			return getPayloadGroup()
		}
		if strings.HasPrefix(fuzztagCode, "{{yak(") || strings.HasPrefix(fuzztagCode, "{{yak:dyn(") {
			return hotPatchSuggestions
		}
		suggestions, _ = _getAllFuzztagSuggestionInfo()
	}

	return suggestions
}

func (s *Server) YaklangLanguageSuggestion(ctx context.Context, req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, error) {
	// check syntaxflow
	if resp, match := SyntaxFlowServer(req); match {
		return resp, nil
	}

	if resp, match := FuzztagServer(req); match {
		return resp, nil
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

	// 添加调试日志
	log.Infof("[LSP Debug] Code: %q, Word: %q, ContainPoint: %v, PointSuffix: %v, Range: Line %d Col %d",
		req.GetYakScriptCode(), word, containPoint, result.PointSuffix,
		req.GetRange().StartLine, req.GetRange().StartColumn)
	if v != nil {
		log.Infof("[LSP Debug] Value Type: %s, IsExternLib: %v, IsUndefined: %v",
			v.GetType(), v.IsExternLib(), v.IsUndefined())
	} else {
		log.Infof("[LSP Debug] Value is nil")
	}

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
	return ret, nil
}

func (s *Server) FuzzTagSuggestion(ctx context.Context, req *ypb.FuzzTagSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, error) {
	ret := &ypb.YaklangLanguageSuggestionResponse{}
	if req.GetInspectType() == HOVER {
		ret.SuggestionMessage = append(ret.SuggestionMessage, fuzztagHover(req.GetFuzztagCode(), "")...)
	} else if req.GetInspectType() == COMPLETION {
		ret.SuggestionMessage = append(ret.SuggestionMessage, fuzztagCompletion(req.GetFuzztagCode(), req.GetHotPatchCode())...)
	}
	return ret, nil
}
