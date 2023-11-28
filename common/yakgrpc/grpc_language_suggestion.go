package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	pta "github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
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

	standardLibrarySuggestions = make([]*ypb.SuggestionDescription, 0, len(doc.DefaultDocumentHelper.Libs))
)

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
			snippets, _ := method.VSCodeSnippets()
			sug := &ypb.SuggestionDescription{
				Label:       methodName,
				Description: method.Description,
				InsertText:  snippets,
				Kind:        "Method",
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
				Description: "Standard Library",
				Kind:        "Module",
			})
		}
	}

	return standardLibrarySuggestions
}

func getFuncDeclByFuncName(totalFuncName string) *yakdoc.FuncDecl {
	libName, funcName := "", totalFuncName
	if strings.Contains(totalFuncName, ".") {
		splited := strings.Split(totalFuncName, ".")
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

func getGolangTypeStringBySSAType(typ ssa.Type) string {
	typStr := typ.String()
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

func getFuncDescByFuncDecl(funcDecl *yakdoc.FuncDecl, typStr string) string {
	desc := fmt.Sprintf("```go\nfunc %s\n```\n\n%s", funcDecl.Decl, funcDecl.Document)
	desc = strings.Replace(desc, "func(", typStr+"(", 1)
	desc = yakdoc.ShrinkTypeVerboseName(desc)
	return desc
}

func getFuncDeclsByWord(word string, containPoint bool) map[string]*yakdoc.FuncDecl {
	if containPoint {
		lib, ok := doc.DefaultDocumentHelper.Libs[strings.Split(word, ".")[0]]
		if !ok {
			return nil
		}
		return lib.Functions
	} else {
		return doc.DefaultDocumentHelper.Functions
	}
}

func getFuncDescByDecls(funcDecls map[string]*yakdoc.FuncDecl, typName string, isStruct bool, tab bool) string {
	desc := ""
	methodNames := lo.MapToSlice(funcDecls, func(methodName string, _ *yakdoc.FuncDecl) string {
		return methodName
	})
	sort.Strings(methodNames)

	for _, methodName := range methodNames {
		funcDecl := funcDecls[methodName]
		funcDesc := ""
		if isStruct {
			funcDesc = fmt.Sprintf("func (%s) %s\n", typName, strings.TrimPrefix(funcDecl.Decl, "func"))
		} else {
			funcDesc = funcDecl.Decl + "\n"
		}
		if tab {
			funcDesc = "    " + funcDesc
		}
		desc += funcDesc
	}

	return desc
}

func getFuncDescBytypeStr(typStr string, typName string, isStruct bool, tab bool) string {
	lib, ok := doc.DefaultDocumentHelper.StructMethods[typStr]
	if !ok {
		return ""
	}

	return getFuncDescByDecls(lib.Functions, typName, isStruct, tab)
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
		if rTyp.KeyTyp.GetTypeKind() == ssa.Bytes {
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
	case ssa.String:
		// string 内置方法
		getStringBuiltinMethodSuggestions()
		m = stringBuiltinMethodSuggestionMap
	}
	sug, ok := m[name]
	if ok {
		return sug.Label, sug.Description
	}
	return
}

func getFuncDeclAndDocBySSAValue(name string, v *ssaapi.Value) (desc string, document string) {
	// 标准库函数
	funcDecl := getFuncDeclByFuncName(name)
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

	// 内置方法
	return getBuiltinFuncDeclAndDoc(lastName, bareTyp)
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
			funcDecl := getFuncDeclByFuncName(name)
			if funcDecl != nil {
				desc = getFuncDescByFuncDecl(funcDecl, typStr)
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
		case ssa.Any:
			// 标准库
			lib, ok := doc.DefaultDocumentHelper.Libs[name]
			if !ok {
				break
			}
			desc = fmt.Sprintf("```go\ntype %s library {\n", name)
			methodDescriptions := getFuncDescByDecls(lib.Functions, typName, false, true)
			desc += methodDescriptions
			desc += "}"
			desc += "\n```"
		}
	} else {
		// ! 这里可能存在value实际上是parent 而不是其本身
		lastName := name[strings.LastIndex(name, ".")+1:]
		if v.IsExtern() {
			// 标准库函数
			funcDecl := getFuncDeclByFuncName(name)
			if funcDecl != nil {
				desc = getFuncDescByFuncDecl(funcDecl, lastName)
			}
		} else {
			// 结构体 / 接口方法
			lib, ok := doc.DefaultDocumentHelper.StructMethods[typStr]
			if ok {
				funcDecl, ok := lib.Functions[lastName]
				if ok {
					desc = getFuncDescByFuncDecl(funcDecl, lastName)
				}
			} else {
				// 内置方法
				decl, document := getBuiltinFuncDeclAndDoc(lastName, bareTyp)
				desc = fmt.Sprintf("```go\nfunc %s\n```\n\n%s", decl, document)
			}
		}
	}

	if desc == "" && !nameContainsPoint {
		desc = fmt.Sprintf("```go\n%s %s\n```", name, typStr)
	}
	return desc
}

func sortValuesByPosition(values ssaapi.Values, position *ssa.Position) ssaapi.Values {
	// todo: 需要修改SSA，需要真正的RefLocation
	values = values.Filter(func(v *ssaapi.Value) bool {
		if v.GetPosition().StartLine > position.StartLine {
			return false
		}
		return true
	})
	sort.SliceStable(values, func(i, j int) bool {
		line1, line2 := values[i].GetPosition().StartLine, values[j].GetPosition().StartLine
		if line1 == line2 {
			return values[i].GetPosition().StartColumn > values[j].GetPosition().StartColumn
		} else {
			return line1 > line2
		}
	})
	return values
}

func getSSAParentValueByPosition(prog *ssaapi.Program, sourceCode string, position *ssa.Position) *ssaapi.Value {
	word := strings.Split(sourceCode, ".")[0]
	values := prog.Ref(word).Filter(func(v *ssaapi.Value) bool {
		if v.GetPosition().StartLine > position.StartLine {
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

func getSSAValueByPosition(prog *ssaapi.Program, sourceCode string, position *ssa.Position) *ssaapi.Value {
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

// todo: 存在如 freq.FuzzCookie 这种拿不到签名，这是因为 SSA 无法找到这个值
func OnHover(prog *ssaapi.Program, req *ypb.YaklangLanguageSuggestionRequest) (ret []*ypb.SuggestionDescription) {
	ret = make([]*ypb.SuggestionDescription, 0)
	position := GrpcRangeToPosition(req.GetRange())
	word, _ := trimSourceCode(position.SourceCode)
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

// todo: 存在如 freq.FuzzCookie 这种拿不到签名，这是因为 SSA 无法找到这个值
func OnSignature(prog *ssaapi.Program, req *ypb.YaklangLanguageSuggestionRequest) (ret []*ypb.SuggestionDescription) {
	ret = make([]*ypb.SuggestionDescription, 0)
	position := GrpcRangeToPosition(req.GetRange())
	word, _ := trimSourceCode(position.SourceCode)
	v := getSSAParentValueByPosition(prog, word, position)
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

	word, containPoint := trimSourceCode(position.SourceCode)
	// 库补全
	if !containPoint {
		ret = append(ret, getStandardLibrarySuggestions()...)
	}

	// 库函数补全
	funcDecls := getFuncDeclsByWord(word, containPoint)
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

	// 结构体成员补全
	if !containPoint {
		return ret
	}

	v := getSSAParentValueByPosition(prog, word, position)
	if v == nil {
		return ret
	}
	bareTyp := ssaapi.GetBareType(v.GetType())
	typStr := getGolangTypeStringBySSAType(bareTyp)
	typName := typStr
	if strings.Contains(typName, ".") {
		typName = typName[strings.LastIndex(typName, ".")+1:]
	}
	switch bareTyp.GetTypeKind() {
	case ssa.StructTypeKind:
		// 结构体成员 / 方法
		rTyp, ok := bareTyp.(*ssa.ObjectType)
		if !ok {
			break
		}
		if rTyp.Combination {
			break
		}

		rTyp.GetMethod()
		for _, key := range rTyp.Keys {
			// 过滤掉非导出字段
			if !shouldExport(key.String()) {
				continue
			}
			keyStr := key.String()
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       keyStr,
				Description: "",
				InsertText:  keyStr,
				Kind:        "Field",
			})
		}

		lib, ok := doc.DefaultDocumentHelper.StructMethods[typStr]
		if !ok {
			return ret
		}

		for methodName, funcDecl := range lib.Functions {
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       methodName,
				Description: funcDecl.Document,
				InsertText:  funcDecl.VSCodeSnippets,
				Kind:        "Method",
			})
		}
	case ssa.InterfaceTypeKind:
		// 接口方法
		lib, ok := doc.DefaultDocumentHelper.StructMethods[typStr]
		if !ok {
			return ret
		}
		for methodName, funcDecl := range lib.Functions {
			ret = append(ret, &ypb.SuggestionDescription{
				Label:       methodName,
				Description: funcDecl.Document,
				InsertText:  funcDecl.VSCodeSnippets,
				Kind:        "Method",
			})
		}
	case ssa.SliceTypeKind:
		// []byte / [] 内置方法
		rTyp, ok := bareTyp.(*ssa.ObjectType)
		if !ok {
			break
		}
		if rTyp.KeyTyp.GetTypeKind() == ssa.Bytes {
			ret = append(ret, getBytesBuiltinMethodSuggestions()...)
		} else {
			ret = append(ret, getSliceBuiltinMethodSuggestions()...)
		}
	case ssa.MapTypeKind:
		// map 内置方法
		ret = append(ret, getMapBuiltinMethodSuggestions()...)
	case ssa.String:
		// string 内置方法
		ret = append(ret, getStringBuiltinMethodSuggestions()...)
	}

	return ret
}

func GrpcRangeToPosition(r *ypb.Range) *ssa.Position {
	return &ssa.Position{
		SourceCode:  r.Code,
		StartLine:   int(r.StartLine),
		StartColumn: int(r.StartColumn - 1),
		EndLine:     int(r.EndLine),
		EndColumn:   int(r.EndColumn - 1),
	}
}

func (s *Server) YaklangLanguageSuggestion(ctx context.Context, req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, error) {
	ret := &ypb.YaklangLanguageSuggestionResponse{}
	prog := ssaapi.Parse(req.YakScriptCode, pta.GetPluginSSAOpt(req.YakScriptType)...)
	if prog == nil {
		return nil, errors.New("ssa parse error")
	}
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
