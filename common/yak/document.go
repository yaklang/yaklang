package yak

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"yaklang/common/log"
	"yaklang/common/yak/antlr4yak"
	"yaklang/common/yak/yakdoc"
	"yaklang/common/yak/yaklang"
	"yaklang/common/yakdocument"
)

func EngineToDocumentHelperWithVerboseInfo(engine yaklang.YaklangEngine) *yakdoc.DocumentHelper {
	helper := &yakdoc.DocumentHelper{
		Libs:      make(map[string]*yakdoc.ScriptLib),
		Functions: make(map[string]*yakdoc.FuncDecl),
		Instances: make(map[string]*yakdoc.LibInstance),
	}

	var extLibs []*yakdoc.ScriptLib
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
					funcDecl := yakdoc.FuncToFuncDecl(name, elementName, value)
					if funcDecl == nil {
						continue
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
					funcDecl := yakdoc.FuncToFuncDecl(globalBanner, name, item)
					helper.Functions[name] = funcDecl
				}
			default:
				helper.Instances[name] = yakdoc.AnyTypeToLibInstance(globalBanner, name, itemType, item)
			}
		}
	}
	return helper
}

func EngineToLibDocuments(engine yaklang.YaklangEngine) []yakdocument.LibDoc {
	var libs []yakdocument.LibDoc

	var globalDoc = yakdocument.LibDoc{
		Name: fmt.Sprintf("%v", "__global__"),
	}

	switch engine.(type) {
	case *antlr4yak.Engine:
		log.Info("loading antlr4yak's completions")
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

			var libDoc = yakdocument.LibDoc{
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
					libDoc.Variables = append(libDoc.Variables, &yakdocument.VariableDoc{
						Name:           fmt.Sprintf("%v.%v", libName, elementName),
						TypeStr:        yakdocument.DumpReflectType(reflect.TypeOf(value)),
						Description:    "//",
						RelativeStruct: structDoc,
					})
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
