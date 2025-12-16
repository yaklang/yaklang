package yakdocument

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

func ReflectFuncToFunctionDoc(libName string, ret reflect.Type) (ExportsFunctionDoc, error) {
	return ReflectFuncToFunctionDocWithCache(libName, ret, nil)
}

// ReflectFuncToFunctionDocWithCache converts reflect.Type to ExportsFunctionDoc with shared cache
// This avoids repeated struct parsing when processing multiple functions
func ReflectFuncToFunctionDocWithCache(libName string, ret reflect.Type, cache *sync.Map) (ExportsFunctionDoc, error) {
	if ret.Kind() != reflect.Func {
		return ExportsFunctionDoc{}, utils.Errorf("no a valid func: %v", ret.Kind())
	}

	if cache == nil {
		cache = new(sync.Map)
	}

	var params []*FieldDoc
	var returnTypes []*FieldDoc
	var structDocs []*StructDoc

	// 设置函数参数的点
	var inParams []reflect.Type
	var isVariadic = ret.IsVariadic()
	for i := range make([]int, ret.NumIn()) {
		p := ret.In(i)
		inParams = append(inParams, p)
	}

	for _index, i := range inParams {
		var rStructName string
		structInfo, _ := Dir(i)
		if structInfo != nil {
			rStructName = StructName(libName, structInfo.PkgPath, structInfo.Name)
			structDocs = append(structDocs, StructHelperToDocWithCache(structInfo, cache)...)
		}

		params = append(params, &FieldDoc{
			Name:               fmt.Sprintf("v%v", _index+1),
			Description:        "",
			TypeStr:            DumpReflectType(i),
			RelativeStructName: rStructName,
			IsVariadic:         _index+1 == len(inParams) && isVariadic,
		})
	}

	// 设置函数返回值文档
	var returns []reflect.Type
	for i := range make([]int, ret.NumOut()) {
		p := ret.Out(i)
		returns = append(returns, p)
	}

	for _index, i := range returns {
		var rStructName string
		structInfo, _ := Dir(i)
		if structInfo != nil {
			rStructName = StructName(libName, structInfo.PkgPath, structInfo.Name)
			structDocs = append(structDocs, StructHelperToDocWithCache(structInfo, cache)...)
		}
		returnTypes = append(returnTypes, &FieldDoc{
			Name:               fmt.Sprintf("r%v", _index),
			Description:        "",
			TypeStr:            DumpReflectType(i),
			RelativeStructName: rStructName,
		})
	}

	var f = ExportsFunctionDoc{
		Name:            "",
		LibName:         "",
		TypeStr:         DumpReflectType(ret),
		Description:     "",
		Params:          params,
		Returns:         returnTypes,
		RelativeStructs: structDocs,
	}
	return f, nil
}

//func EngineToLibDocuments(engine *yaklang.YakEngine) []LibDoc {
//	var libs []LibDoc
//
//	for libName, item := range engine.GetFntable() {
//		iTy := reflect.TypeOf(item)
//		iVl := reflect.ValueOf(item)
//		_, _ = iTy, iVl
//
//		switch iTy {
//		case reflect.TypeOf(make(map[string]interface{})):
//			res := item.(map[string]interface{})
//			if res == nil && len(res) <= 0 {
//				continue
//			}
//
//			var libDoc = LibDoc{
//				Name: fmt.Sprintf("%v", libName),
//			}
//			for elementName, value := range res {
//				switch methodType := reflect.TypeOf(value); methodType.Kind() {
//				case reflect.Func:
//					fDoc, err := ReflectFuncToFunctionDoc(methodType)
//					if err != nil {
//						continue
//					}
//					fDoc.LibName = libName
//					fDoc.Name = fmt.Sprintf("%v.%v", libName, elementName)
//					libDoc.Functions = append(libDoc.Functions, &fDoc)
//					sort.SliceStable(libDoc.Functions, func(i, j int) bool {
//						return libDoc.Functions[i].Name < libDoc.Functions[j].Name
//					})
//				default:
//					var structDoc []*StructDoc
//					s, _ := Dir(value)
//					if s != nil {
//						structDoc = StructHelperToDoc(s)
//					}
//					libDoc.Variables = append(libDoc.Variables, &VariableDoc{
//						Name:           fmt.Sprintf("%v.%v", libName, elementName),
//						TypeStr:        DumpReflectType(reflect.TypeOf(value)),
//						Description:    "//",
//						RelativeStruct: structDoc,
//					})
//					sort.SliceStable(libDoc.Variables, func(i, j int) bool {
//						return libDoc.Variables[i].Name < libDoc.Variables[j].Name
//					})
//				}
//			}
//
//			libs = append(libs, libDoc)
//		default:
//			if iTy == nil {
//				continue
//			}
//
//			globalBanner := "__GLOBAL__"
//			_ = globalBanner
//			switch iTy.Kind() {
//			case reflect.Func:
//				if strings.HasPrefix(libName, "$") || strings.HasPrefix(libName, "_") {
//					//helper.BuildInFunctions[name] = funcTypeToPalmScriptLibFunc(globalBanner, name, iTy)
//				} else {
//					//helper.UserFunctions[name] = funcTypeToPalmScriptLibFunc(globalBanner, name, iTy)
//				}
//			default:
//				//helper.Instances[name] = anyTypeToPalmScriptLibInstance(globalBanner, name, iTy)
//			}
//		}
//	}
//	return libs
//}

func LibsToRelativeStructs(libs ...LibDoc) []*StructDocForYamlMarshal {
	var structs []string
	var structsMap = map[string]*StructDoc{}

	var loadStructs = func(all []*StructDoc, libName string) {
		for _, s := range all {
			if s == nil {
				return
			}
			if utils.StringArrayContains(structs, s.StructName) {
				return
			}
			s.LibName = libName
			structs = append(structs, s.StructName)
			structsMap[s.StructName] = s
		}
	}

	for _, lib := range libs {
		for _, i := range lib.Variables {
			loadStructs(i.RelativeStruct, lib.Name)
		}

		for _, f := range lib.Functions {
			loadStructs(f.RelativeStructs, lib.Name)
		}
	}

	sort.Strings(structs)
	var res []*StructDocForYamlMarshal
	for _, i := range structs {
		stct := structsMap[i]
		ret := &StructDocForYamlMarshal{
			IsBuildInStruct: stct.IsBuildInLib(),
			LibName:         stct.LibName,
			StructName:      stct.StructName,
			Description:     stct.Description,
			Fields:          stct.Fields,
			MethodsDoc:      stct.MethodsDoc,
			PtrMethodDoc:    stct.PtrMethodDoc,
		}
		if ret.StructName == "" || ret.StructName == "." {
			continue
		}
		res = append(res, ret)
	}

	return res
}
