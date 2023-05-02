package yakdocument

import (
	"encoding/json"
	"fmt"
	"sort"
	"yaklang/common/utils"
)

type YakLibDocCompletion struct {
	LibName            string                  `json:"libName"`
	Prefix             string                  `json:"prefix"`
	FunctionCompletion []YakFunctionCompletion `json:"functions"`
}

type FieldsCompletion struct {
	IsMethod                 bool   `json:"isMethod"`
	FieldName                string `json:"fieldName"`
	FieldTypeVerbose         string `json:"fieldTypeVerbose"`
	LibName                  string `json:"libName"`
	StructName               string `json:"structName"`
	StructNameShort          string `json:"structNameShort"`
	MethodsCompletion        string `json:"methodsCompletion"`
	MethodsCompletionVerbose string `json:"methodsCompletionVerbose"`
	IsGolangBuildOrigin      bool   `json:"isGolangBuildOrigin"`
}

type YakFunctionCompletion struct {
	Function         string `json:"functionName"`
	FunctionDocument string `json:"document"`
	DefinitionStr    string `json:"definitionStr"`
}

type YakCompletion struct {
	LibNames              []string                      `json:"libNames"`
	LibCompletions        []YakLibDocCompletion         `json:"libCompletions"`
	FieldsCompletions     []FieldsCompletion            `json:"fieldsCompletions"`
	LibToFieldCompletions map[string][]FieldsCompletion `json:"libToFieldCompletions"`
}

func (y *YakCompletion) sort() {
	sort.Strings(y.LibNames)
	sort.SliceStable(y.LibCompletions, func(i, j int) bool {
		return y.LibCompletions[i].LibName > y.LibCompletions[j].LibName
	})
	for _, f := range y.LibCompletions {
		sort.SliceStable(f.FunctionCompletion, func(i, j int) bool {
			return f.FunctionCompletion[i].Function > f.FunctionCompletion[j].Function
		})
	}
	sort.SliceStable(y.FieldsCompletions, func(i, j int) bool {
		return y.FieldsCompletions[i].FieldName > y.FieldsCompletions[j].FieldName
	})
}

func LibDocsToCompletionJson(libs ...LibDoc) ([]byte, error) {
	return LibDocsToCompletionJsonEx(true, libs...)
}

func LibDocsToCompletionJsonShort(libs ...LibDoc) ([]byte, error) {
	return LibDocsToCompletionJsonEx(false, libs...)
}

var whiteStructListGlob = []string{}
var blackStructListGlob = []string{}

func LibDocsToCompletionJsonEx(all bool, libs ...LibDoc) ([]byte, error) {
	sort.SliceStable(libs, func(i, j int) bool {
		return libs[i].Name < libs[j].Name
	})

	var yakComp YakCompletion
	var comps []YakLibDocCompletion
	var libName []string
	for _, l := range libs {
		libName = append(libName, l.Name)
		var libComp = YakLibDocCompletion{
			LibName: l.Name,
			Prefix:  fmt.Sprintf("%v.", l.Name),
		}
		for _, fIns := range l.Functions {
			libComp.FunctionCompletion = append(libComp.FunctionCompletion, YakFunctionCompletion{
				Function:         fIns.CompletionStr(),
				FunctionDocument: fIns.Description,
				DefinitionStr:    fIns.DefinitionStr(),
			})
		}
		comps = append(comps, libComp)
	}
	yakComp.LibNames = libName
	yakComp.LibCompletions = comps
	yakComp.LibToFieldCompletions = make(map[string][]FieldsCompletion)

	structs := LibsToRelativeStructs(libs...)
	for _, stct := range structs {
		if !all {
			// 黑名单过滤不想要的内容
			if utils.MatchAnyOfGlob(stct.StructName, blackStructListGlob...) {
				continue
			}

			// 判断是否要过滤一些不重要的数据？
			if stct.IsBuildInLib() {
				// 如果是内置库的话，需要判断是不是符合白名单
				if len(whiteStructListGlob) > 0 {
					if !utils.MatchAnyOfGlob(stct.StructName, whiteStructListGlob...) {
						continue
					}
				} else {
					continue
				}
			}
		}

		for _, compl := range stct.GenerateCompletion() {
			if compl.LibName == "" {
				yakComp.FieldsCompletions = append(yakComp.FieldsCompletions, compl)
			} else {
				yakComp.LibToFieldCompletions[compl.LibName] = append(yakComp.LibToFieldCompletions[compl.LibName], compl)
			}
		}
	}

	yakComp.sort()
	return json.MarshalIndent(yakComp, "", "  ")
}
