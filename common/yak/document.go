package yak

import (
	"container/list"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yakdocument"
)

type EmbedFieldTypeAndMethod struct {
	FieldType reflect.Type
	Method    reflect.Method
}

func ClearHelper(helper *yakdoc.DocumentHelper) {
	clearFieldParamsType := func(funcs map[string]*yakdoc.FuncDecl) {
		for _, funcDecl := range funcs {
			for _, param := range funcDecl.Params {
				param.RefType = nil
			}
			for _, result := range funcDecl.Results {
				result.RefType = nil
			}
		}
	}

	for _, lib := range helper.Libs {
		clearFieldParamsType(lib.Functions)
	}
	for _, lib := range helper.StructMethods {
		clearFieldParamsType(lib.Functions)
	}
	clearFieldParamsType(helper.Functions)
}

func EngineToDocumentHelperWithVerboseInfo(engine *antlr4yak.Engine) *yakdoc.DocumentHelper {
	helper := &yakdoc.DocumentHelper{
		Libs:          make(map[string]*yakdoc.ScriptLib),
		Functions:     make(map[string]*yakdoc.FuncDecl),
		Instances:     make(map[string]*yakdoc.LibInstance),
		StructMethods: make(map[string]*yakdoc.ScriptLib),
	}

	var extLibs []*yakdoc.ScriptLib
	// 标准库导出的函数
	for name, item := range engine.GetFntable() {
		itemType := reflect.TypeOf(item)
		itemValue := reflect.ValueOf(item)
		_, _ = itemType, itemValue

		switch itemType {
		case reflect.TypeOf(make(map[string]interface{})):
			res := item.(map[string]interface{})
			if res == nil && len(res) <= 0 {
				continue
			}

			extLib := &yakdoc.ScriptLib{
				Name:      name,
				Functions: make(map[string]*yakdoc.FuncDecl),
			}
			extLibs = append(extLibs, extLib)
			helper.Libs[extLib.Name] = extLib

			for elementName, value := range res {
				switch methodType := reflect.TypeOf(value); methodType.Kind() {
				case reflect.Func:
					funcDecl, err := yakdoc.FuncToFuncDecl(value, name, elementName)
					if err != nil {
						log.Warnf("failed to get func decl from %s.%s: %v", name, elementName, err)
						funcDecl = &yakdoc.FuncDecl{}
					}
					extLib.Functions[elementName] = funcDecl
					extLib.ElementDocs = append(extLib.ElementDocs, funcDecl.String())
				default:
					item := yakdoc.AnyTypeToLibInstance(
						extLib.Name, elementName,
						methodType, value,
					)
					extLib.LibsInstances = append(extLib.LibsInstances, item)
					extLib.ElementDocs = append(extLib.ElementDocs, item.String())
				}
			}
			sort.Strings(extLib.ElementDocs)
		default:
			if itemType == nil {
				continue
			}
			globalBanner := "__GLOBAL__"
			switch itemType.Kind() {
			case reflect.Func:
				if !strings.HasPrefix(name, "$") && !strings.HasPrefix(name, "_") {
					funcDecl, err := yakdoc.FuncToFuncDecl(item, globalBanner, name)
					if err != nil {
						log.Warnf("failed to get func decl from %s.%s: %v", globalBanner, name, err)
						funcDecl = &yakdoc.FuncDecl{}
					}
					helper.Functions[name] = funcDecl
				}
			default:
				helper.Instances[name] = yakdoc.AnyTypeToLibInstance(globalBanner, name, itemType, item)
			}
		}
	}
	// 标准库可能会返回的结构体的方法
	funcTypes := make([]reflect.Type, 0)

	var getTypeFromReflectFunctionType func(typ reflect.Type, level int) []reflect.Type

	getFuncTypesFromFuncDecl := func(decl *yakdoc.FuncDecl) []reflect.Type {
		var types []reflect.Type
		for _, param := range decl.Params {
			types = append(types, param.RefType)
		}
		for _, result := range decl.Results {
			types = append(types, result.RefType)
		}
		return types
	}
	getTypeFromReflectFunctionType = func(typ reflect.Type, level int) []reflect.Type {
		ret := make([]reflect.Type, 0)
		if typ.Kind() != reflect.Func {
			return ret
		}
		if level >= 2 {
			return ret
		}
		for i := 0; i < typ.NumIn(); i++ {
			ret = append(ret, typ.In(i))
		}
		for i := 0; i < typ.NumOut(); i++ {
			ret = append(ret, typ.Out(i))
		}
		for _, t := range ret {
			ret = append(ret, getTypeFromReflectFunctionType(t, level+1)...)
		}
		return ret
	}
	pushBackWithoutNil := func(list *list.List, typ reflect.Type) {
		if !utils.IsNil(typ) {
			list.PushBack(typ)
		}
	}

	for _, lib := range extLibs {
		for _, funcDecl := range lib.Functions {
			funcTypes = append(funcTypes, getFuncTypesFromFuncDecl(funcDecl)...)
		}
	}
	for _, funcDecl := range helper.Functions {
		funcTypes = append(funcTypes, getFuncTypesFromFuncDecl(funcDecl)...)
	}

	funcTypes = lo.Uniq(funcTypes)
	filter := make(map[reflect.Type]struct{}, 0)

	funcTypesList := list.New()
	for _, typ := range funcTypes {
		pushBackWithoutNil(funcTypesList, typ)
	}

	for iTyp := funcTypesList.Back(); iTyp != nil; iTyp = funcTypesList.Back() {
		funcTypesList.Remove(iTyp)

		typ := iTyp.Value.(reflect.Type)

		if _, ok := filter[typ]; ok {
			continue
		}
		filter[typ] = struct{}{}

		structName := ""
		pkgPath := ""
		isStruct := false

		for {
			typKind := typ.Kind()
			if typKind == reflect.Slice || typKind == reflect.Array || typKind == reflect.Chan {
				typ = typ.Elem()
			} else {
				break
			}
		}

		typKind := typ.Kind()
		if typKind == reflect.Struct || typKind == reflect.Interface {
			isStruct = true
			pkgPath = typ.PkgPath()
			structName = typ.Name()

		} else if typKind == reflect.Ptr {
			isStruct = typ.Elem().Kind() == reflect.Struct
			pkgPath = typ.Elem().PkgPath()
			structName = typ.Elem().Name()
		} else if typKind == reflect.Func {
			// 形如 (s *Struct) MethodName() (callback func(*Struct2)) {}
			// 需要递归再获取类型
			for _, newTyp := range getTypeFromReflectFunctionType(typ, 0) {
				pushBackWithoutNil(funcTypesList, newTyp)
			}
		}

		if structName != "" && isStruct {
			//
			if typKind == reflect.Interface {
				log.Warnf("need inject interface document for: %s", structName)
			}

			structName = fmt.Sprintf("%s.%s", pkgPath, structName)
			lib := &yakdoc.ScriptLib{
				Name:      structName,
				Functions: make(map[string]*yakdoc.FuncDecl),
			}
			for i := 0; i < typ.NumMethod(); i++ {
				method := typ.Method(i)
				methodName := method.Name
				// 对于方法中的参数和返回值，需要递归再获取类型
				for _, newTyp := range getTypeFromReflectFunctionType(method.Type, 0) {
					pushBackWithoutNil(funcTypesList, newTyp)
				}

				// 为了处理 embed 字段，其组合了匿名结构体字段的方法
				EmbedFieldAndMethodList := list.New()
				EmbedFieldAndMethodList.PushBack(&EmbedFieldTypeAndMethod{
					FieldType: typ,
					Method:    method,
				})
				for item := EmbedFieldAndMethodList.Back(); item != nil; item = EmbedFieldAndMethodList.Back() {
					EmbedFieldAndMethodList.Remove(item)

					fieldTypeAndMethod := item.Value.(*EmbedFieldTypeAndMethod)
					fieldType, method := fieldTypeAndMethod.FieldType, fieldTypeAndMethod.Method
					// 如果是指针类型，那么需要获取其指向的类型，如果不是结构体类型，那么就不需要处理
					if fieldType.Kind() == reflect.Ptr {
						fieldType = fieldType.Elem()
						if fieldType.Kind() != reflect.Struct {
							continue
						}
					}

					var (
						err      error
						funcDecl *yakdoc.FuncDecl
					)

					f := method.Func
					if !f.IsValid() {
						// ? 匿名字段是一个匿名接口，例如继承了 net.Conn 接口, fallback 处理
						methodTyp := method.Type

						funcDecl := &yakdoc.FuncDecl{
							LibName:    structName,
							MethodName: methodName,
							Document:   "",
							Decl:       strings.Replace(methodTyp.String(), "func(", methodName+"(", 1),
							Params:     make([]*yakdoc.Field, 0, methodTyp.NumIn()),
							Results:    make([]*yakdoc.Field, 0, methodTyp.NumOut()),
						}
						paramsStr := make([]string, 0, methodTyp.NumIn())
						for i := 0; i < methodTyp.NumIn(); i++ {
							paramTyp := methodTyp.In(i)
							param := &yakdoc.Field{
								Name:    paramTyp.Name(),
								Type:    paramTyp.String(),
								RefType: paramTyp,
							}
							funcDecl.Params = append(funcDecl.Params, param)

							if strings.HasPrefix(param.Type, "...") {
								paramsStr = append(paramsStr, fmt.Sprintf("${%v:%v...}", i+1, param.Name))
							} else {
								if param.Type == "any" || param.Type == "interface{}" {
									paramsStr = append(paramsStr, fmt.Sprintf("${%v:%v}", i+1, param.Name))
								} else {
									paramsStr = append(paramsStr, fmt.Sprintf("${%v:%v /*type: %v*/}", i+1, param.Name, param.Type))
								}
							}
						}
						for i := 0; i < methodTyp.NumOut(); i++ {
							resultTyp := methodTyp.Out(i)
							result := &yakdoc.Field{
								Name:    resultTyp.Name(),
								Type:    resultTyp.String(),
								RefType: resultTyp,
							}
							funcDecl.Results = append(funcDecl.Results, result)
						}
						// 生成 vscode 补全
						funcDecl.VSCodeSnippets = fmt.Sprintf("%s(%s)", methodName, strings.Join(paramsStr, ", "))
						lib.Functions[methodName] = funcDecl
					} else {
						funcDecl, err = yakdoc.FuncToFuncDecl(f.Interface(), structName, methodName)
						if err == nil {
							lib.Functions[methodName] = funcDecl
							break
						} else if errors.Is(err, yakdoc.ErrAutoGenerated) {
							// 如果是自动生成的代码，那么就是匿名结构体字段
							// 需要递归获取匿名结构体字段的方法
							for j := 0; j < fieldType.NumField(); j++ {
								field := fieldType.Field(j)

								fieldTyp := field.Type

								if !field.Anonymous {
									continue
								}
								m, ok := fieldTyp.MethodByName(methodName)
								if !ok {
									continue
								}
								EmbedFieldAndMethodList.PushBack(&EmbedFieldTypeAndMethod{
									FieldType: fieldTyp,
									Method:    m,
								})
							}
						} else {
							log.Warnf("failed to get func decl from %s.%s: %v", structName, methodName, err)
							break
						}
					}
				}
			}
			helper.StructMethods[structName] = lib
		}

	}

	// 调用回调，注入一些其他的函数注释
	helper.Callback()
	ClearHelper(helper)
	return helper
}

// ! 老接口
func EngineToLibDocuments(engine *antlr4yak.Engine) []yakdocument.LibDoc {
	var libs []yakdocument.LibDoc

	globalDoc := yakdocument.LibDoc{
		Name: fmt.Sprintf("%v", "__global__"),
	}

	fnTable := engine.GetFntable()
	for libName, item := range fnTable {
		iTy := reflect.TypeOf(item)
		iVl := reflect.ValueOf(item)
		_, _ = iTy, iVl

		switch iTy {
		case reflect.TypeOf(make(map[string]interface{})):
			res := item.(map[string]interface{})
			if res == nil && len(res) <= 0 {
				continue
			}

			libDoc := yakdocument.LibDoc{
				Name: fmt.Sprintf("%v", libName),
			}
			for elementName, value := range res {
				switch methodType := reflect.TypeOf(value); methodType.Kind() {
				case reflect.Func:
					fDoc, err := yakdocument.ReflectFuncToFunctionDoc(libName, methodType)
					if err != nil {
						continue
					}
					fDoc.LibName = libName
					fDoc.Name = fmt.Sprintf("%v.%v", libName, elementName)
					libDoc.Functions = append(libDoc.Functions, &fDoc)
					sort.SliceStable(libDoc.Functions, func(i, j int) bool {
						return libDoc.Functions[i].Name < libDoc.Functions[j].Name
					})
				default:
					var structDoc []*yakdocument.StructDoc
					s, _ := yakdocument.Dir(value)
					if s != nil {
						structDoc = yakdocument.StructHelperToDoc(s)
					}
					varDoc := &yakdocument.VariableDoc{
						Name:           fmt.Sprintf("%v.%v", libName, elementName),
						TypeStr:        yakdocument.DumpReflectType(reflect.TypeOf(value)),
						Description:    "//",
						RelativeStruct: structDoc,
					}
					if utils.MatchAnyOfGlob(varDoc.TypeStr, "*int*") {
						varDoc.ValueVerbose = fmt.Sprintf("0x%x", value)
					} else if utils.MatchAnyOfSubString(varDoc.TypeStr, "*str*") {
						varDoc.ValueVerbose = fmt.Sprintf("%q", value)
					}

					libDoc.Variables = append(libDoc.Variables, varDoc)
					sort.SliceStable(libDoc.Variables, func(i, j int) bool {
						return libDoc.Variables[i].Name < libDoc.Variables[j].Name
					})
				}
			}

			libs = append(libs, libDoc)
		default:
			if iTy == nil {
				continue
			}

			key := libName
			value := item
			_, _ = key, value
			if strings.HasPrefix(libName, "$") || strings.HasPrefix(libName, "_") {
				continue
			}
			switch iTy.Kind() {
			case reflect.Func:
				fDoc, err := yakdocument.ReflectFuncToFunctionDoc(libName, iTy)
				if err != nil {
					continue
				}
				fDoc.LibName = globalDoc.Name
				fDoc.Name = key
				globalDoc.Functions = append(globalDoc.Functions, &fDoc)
				sort.SliceStable(globalDoc.Functions, func(i, j int) bool {
					return globalDoc.Functions[i].Name < globalDoc.Functions[j].Name
				})
			default:
				globalDoc.Variables = append(globalDoc.Variables, &yakdocument.VariableDoc{
					Name:        key,
					TypeStr:     yakdocument.DumpReflectType(reflect.TypeOf(value)),
					Description: "//",
				})
				sort.SliceStable(globalDoc.Variables, func(i, j int) bool {
					return globalDoc.Variables[i].Name < globalDoc.Variables[j].Name
				})
			}
		}
	}
	libs = append(libs, globalDoc)
	return libs
}
