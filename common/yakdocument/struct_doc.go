package yakdocument

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"yaklang/common/log"
	"yaklang/common/yak/yakdoc/doc"
	"yaklang/common/yak/yaklib/codec"
)

func StructName(libName, pkgPath, name string) string {
	if libName == "" {
		return fmt.Sprintf("%v.%v", pkgPath, name)
	}
	return fmt.Sprintf("%v.%v", pkgPath, name)
}

type FieldDoc struct {
	StructName         string `yaml:"structname,omitempty"`
	Name               string
	ParamAlias         string `yaml:"param_alias"`
	TypeAlias          string `yaml:"type_alias"`
	Description        string
	RelativeStructName string `yaml:"relative_structname,omitempty"`
	TypeStr            string `yaml:"type_str"`
	IsVariadic         bool   `yaml:"is_variadic,omitempty"`
}

func (f *FieldDoc) Hash() string {
	return codec.Sha512(fmt.Sprint(f.StructName, f.TypeStr, f.Name))
}

func (f *FieldDoc) NameVerbose() string {
	if f.ParamAlias != "" {
		return f.ParamAlias
	}
	return f.Name
}

func (f *FieldDoc) TypeVerbose() string {
	if f.TypeAlias != "" {
		return f.TypeAlias
	}
	return typeStrFix(f.TypeStr, f.IsVariadic)
}

type MethodDoc struct {
	Ptr                bool `yaml:"ptr,omitempty"`
	Description        string
	StructName         string
	Name               string
	Params             []*FieldDoc  `yaml:"params,omitempty"`
	Returns            []*FieldDoc  `yaml:"returns,omitempty"`
	RelativeStructName []*StructDoc `yaml:"relative_struct_name"`
}

func (m *MethodDoc) Hash() string {
	var paramsHash []string
	var returnHash []string
	for _, p := range m.Params {
		paramsHash = append(paramsHash, p.Hash())
	}
	for _, r := range m.Returns {
		returnHash = append(returnHash, r.Hash())
	}
	sort.Strings(paramsHash)
	sort.Strings(returnHash)
	return codec.Sha512(fmt.Sprint(
		m.Ptr, m.StructName, m.Name,
		//strings.Join(paramsHash, "|"),
		//strings.Join(returnHash, "|"),
	))
}

type StructDoc struct {
	StructName      string
	IsBuildInStruct bool
	LibName         string       `yaml:"-"`
	Description     string       `yaml:"-"`
	Fields          []*FieldDoc  `yaml:"-"`
	MethodsDoc      []*MethodDoc `yaml:"-"`
	PtrMethodDoc    []*MethodDoc `yaml:"-"`
}

func (s *StructDoc) IsBuildInLib() bool {
	return (&StructDocForYamlMarshal{StructName: s.StructName}).IsBuildInLib()
}

type StructDocForYamlMarshal struct {
	IsBuildInStruct bool         `yaml:"is_build_in_struct"`
	LibName         string       `json:"lib_name"`
	StructName      string       `yaml:"struct_name"`
	Description     string       `yaml:"description"`
	Fields          []*FieldDoc  `yaml:"fields"`
	MethodsDoc      []*MethodDoc `yaml:"methods"`
	PtrMethodDoc    []*MethodDoc `yaml:"ptr_methods"`
}

func (s *StructDocForYamlMarshal) GenerateCompletion() []FieldsCompletion {
	var res []FieldsCompletion
	var structNameShort string
	sl := strings.SplitN(s.StructName, ".", 2)
	if len(sl) > 1 {
		structNameShort = sl[1]
	} else {
		structNameShort = s.StructName
	}
	for _, f := range s.Fields {
		res = append(res, FieldsCompletion{
			IsMethod:            false,
			FieldName:           f.Name,
			FieldTypeVerbose:    f.TypeVerbose(),
			LibName:             s.LibName,
			StructName:          f.StructName,
			StructNameShort:     structNameShort,
			IsGolangBuildOrigin: s.IsBuildInStruct,
		})
	}

	methodDocToCompletion := func(m *MethodDoc) string {
		var paramStrs []string
		for index, param := range m.Params {
			paramStrs = append(paramStrs, fmt.Sprintf("${%d:%v /*type: %v*/}", index+1, param.NameVerbose(), param.TypeVerbose()))
		}
		result := fmt.Sprintf("%v(%v)", m.Name, strings.Join(paramStrs, ", "))

		return result
	}

	methodDocToCompletionVerbose := func(m *MethodDoc) string {
		var paramStrs []string
		for _, param := range m.Params {
			paramStrs = append(paramStrs, fmt.Sprintf("%v", param.NameVerbose()))
		}
		result := fmt.Sprintf("%v(%v)", m.Name, strings.Join(paramStrs, ", "))

		return result
	}

	for _, f := range s.MethodsDoc {
		res = append(res, FieldsCompletion{
			IsMethod:                 true,
			LibName:                  s.LibName,
			FieldName:                f.Name,
			StructName:               f.StructName,
			StructNameShort:          structNameShort,
			MethodsCompletion:        methodDocToCompletion(f),
			MethodsCompletionVerbose: methodDocToCompletionVerbose(f),
			IsGolangBuildOrigin:      s.IsBuildInStruct,
		})
	}

	for _, f := range s.PtrMethodDoc {
		res = append(res, FieldsCompletion{
			IsMethod:                 true,
			FieldName:                f.Name,
			LibName:                  s.LibName,
			StructName:               f.StructName,
			StructNameShort:          structNameShort,
			MethodsCompletion:        methodDocToCompletion(f),
			MethodsCompletionVerbose: methodDocToCompletionVerbose(f),
			IsGolangBuildOrigin:      s.IsBuildInStruct,
		})
	}

	return res
}

func (s *StructDocForYamlMarshal) IsBuildInLib() bool {
	if s.StructName == "." || s.StructName == "" {
		return true
	}
	for _, r := range []string{
		"palm/",
	} {
		if strings.HasPrefix(s.StructName, r) {
			return false
		}
	}
	return true
}

func (s *StructDocForYamlMarshal) Merge(e *StructDocForYamlMarshal) {
	s.Description = e.Description
	for _, out := range s.Fields {
		for _, sub := range e.Fields {
			if out.Hash() == sub.Hash() {
				// 是同一个字段
				out.Description = sub.Description
				out.ParamAlias = sub.ParamAlias
				out.TypeAlias = sub.TypeAlias
			}
		}
	}

	for _, out := range s.MethodsDoc {
		for _, sub := range e.MethodsDoc {
			if out.StructName+out.Name == sub.StructName+sub.Name {
			}
			if out.Hash() == sub.Hash() {
				// 是同一个字段
				out.Description = sub.Description

				for _, r1 := range out.Returns {
					for _, sr1 := range sub.Returns {
						if r1.Hash() == sr1.Hash() {
							// 是同一个字段
							r1.Description = sr1.Description
							r1.ParamAlias = sr1.ParamAlias
							r1.TypeAlias = sr1.TypeAlias
						}
					}
				}

				for _, r1 := range out.Params {
					for _, sr1 := range sub.Params {
						if r1.Hash() == sr1.Hash() {
							// 是同一个字段
							r1.Description = sr1.Description
							r1.ParamAlias = sr1.ParamAlias
							r1.TypeAlias = sr1.TypeAlias
						}
					}
				}
			}
		}
	}
}

type ExportsFunctionDoc struct {
	Name            string
	LibName         string `yaml:"-"`
	TypeStr         string `yaml:"type_str"`
	LongDescription string `yaml:"long_description"`
	Description     string
	Example         string       `yaml:"example,omitempty"`
	Params          []*FieldDoc  `yaml:"params,omitempty"`
	Returns         []*FieldDoc  `yaml:"returns,omitempty"`
	RelativeStructs []*StructDoc `yaml:"relative_structs,omitempty"`
}

func (e *ExportsFunctionDoc) Hash() string {
	return codec.Sha512(fmt.Sprint(e.Name, e.TypeStr))
}

func (e *ExportsFunctionDoc) Fragment() string {
	return strings.ToLower(strings.ReplaceAll(e.Name, ".", ""))
}

func (e *ExportsFunctionDoc) DefinitionStr() string {
	if def := doc.Document.LibFuncDefinitionStr(e.LibName, e.Name); def != "" {
		return def
	}

	if strings.HasPrefix(e.Name, e.LibName+".") {
		if ret := doc.Document.LibFuncDefinitionStr(e.LibName, e.Name[len(e.LibName+"."):]); ret != "" {
			return ret
		}
	}

	var paramStr []string
	for _, p := range e.Params {
		name := p.ParamAlias
		if name == "" {
			name = p.Name
		}
		if p.IsVariadic {
			paramStr = append(paramStr, fmt.Sprintf("%v %v", name, p.TypeVerbose()))
		} else {
			paramStr = append(paramStr, fmt.Sprintf("%v: %v", name, p.TypeVerbose()))
		}
	}

	var returnStr []string
	for _, p := range e.Returns {
		name := p.ParamAlias
		if name == "" {
			name = p.Name
		}
		returnStr = append(returnStr, fmt.Sprintf("%v: %v", name, p.TypeVerbose()))
	}

	if returnStr == nil {
		return fmt.Sprintf("`func %v(%v)`", e.Name, strings.Join(paramStr, ", "))
	}
	return fmt.Sprintf(
		"func %v(%v) return (%v)",
		e.Name, strings.Join(paramStr, ", "),
		strings.Join(returnStr, ", "),
	)
}

func typeStrFix(raw string, isVariadic bool) string {
	if isVariadic {
		if strings.HasPrefix(raw, "[]") {
			raw = fmt.Sprintf("...%v", raw[2:])
		}
	}
	for _, a := range [][2]string{
		{"interface{}", "any"},
		{"interface {}", "any"},
		{"[]uint8", "bytes"},
		{"uint8", "byte"},
	} {
		raw = strings.ReplaceAll(raw, a[0], a[1])
	}
	return raw
}

func (e *ExportsFunctionDoc) CompletionStr() string {
	if ret := doc.Document.LibFuncAutoCompletion(e.LibName, e.Name); ret != "" {
		return ret
	}

	if strings.HasPrefix(e.Name, e.LibName+".") {
		if ret := doc.Document.LibFuncAutoCompletion(e.LibName, e.Name[len(e.LibName+"."):]); ret != "" {
			return ret
		}
	}

	var paramStr []string
	for index, p := range e.Params {
		name := p.ParamAlias
		if name == "" {
			name = p.Name
		}

		if p.IsVariadic {
			s := p.TypeVerbose()
			if strings.HasPrefix(s, "...") {
				s = s[3:]
			}
			paramStr = append(paramStr, fmt.Sprintf("${%v:%v/*type ...%v*/}", index+1, name, s))
		} else {
			paramStr = append(paramStr, fmt.Sprintf("${%v:%v/*type: %v*/}", index+1, name, p.TypeVerbose()))
		}
	}

	funcName := e.Name
	funcName = strings.ReplaceAll(funcName, e.LibName+".", "")
	return fmt.Sprintf("%v(%v)", funcName, strings.Join(paramStr, ", "))
}

func (e *ExportsFunctionDoc) Merge(target *ExportsFunctionDoc) {
	e.Description = target.Description
	e.LongDescription = target.LongDescription
	e.Example = target.Example
	e.Params = target.Params
	e.Returns = target.Returns
}

type VariableDoc struct {
	Name           string
	TypeStr        string
	Description    string
	RelativeStruct []*StructDoc `yaml:"relative_struct,omitempty"`
}

func (e *VariableDoc) Hash() string {
	return codec.Sha512(fmt.Sprint(e.Name, e.TypeStr))
}

type LibDoc struct {
	Name      string
	Functions []*ExportsFunctionDoc
	Variables []*VariableDoc
}

func (l *LibDoc) Hash() string {
	var s []string
	s = append(s, l.Name)

	for _, f := range l.Functions {
		s = append(s, f.Hash())
	}

	for _, f := range l.Variables {
		s = append(s, f.Hash())
	}

	sort.Strings(s)
	return codec.Sha512(strings.Join(s, "|"))
}

func (l *LibDoc) Merge(e *LibDoc) {
	for _, nowFunc := range l.Functions {
		for _, existedFunc := range e.Functions {
			if nowFunc.Hash() == existedFunc.Hash() {
				nowFunc.Description = existedFunc.Description
				nowFunc.Merge(existedFunc)
				break
			}
		}
	}

	for _, nowVar := range l.Variables {
		for _, eVar := range e.Variables {
			if nowVar.Hash() == eVar.Hash() {
				nowVar.Description = eVar.Description
				break
			}
		}
	}
}

var yaklibMarkdownStructDoc = fmt.Sprintf(`# {{ .Name }}`)

var yaklibMarkdownAPIDoc = fmt.Sprintf(`# {{ .Name }}

{{ if eq (len .Functions ) 0 }} {{ else }}
|成员函数|函数描述/介绍|
|:------|:--------|
{{ range .Functions }} | [{{ .Name }}](#{{ .Fragment }}) | {{ .Description }} |
{{end}}
{{ end }}


{{ if eq (len .Variables) 0 }} {{ else }}## 变量定义

|变量调用名|变量类型|变量解释/帮助信息|
|:-----------|:---------- |:-----------|
{{ range .Variables}}|%v{{ .Name }}%v|%v{{ .TypeStr }}%v| {{.Description}}|
{{end}}
{{end}}


{{ if eq (len .Functions ) 0 }} {{ else }}
## 函数定义
{{ range .Functions }}
### {{ .Name }}

{{ .Description }}

#### 详细描述

{{ .LongDescription | html }}

#### 定义：

%v{{ .DefinitionStr }}%v

{{ if eq (len .Params) 0 }} {{else}}
#### 参数

|参数名|参数类型|参数解释|
|:-----------|:---------- |:-----------|
{{ range .Params }}| {{ .NameVerbose }} | %v{{ .TypeVerbose }}%v |  {{ .Description }} |
{{ end }}

{{end}}

{{ if eq (len .Returns) 0 }} {{else}}
#### 返回值

|返回值(顺序)|返回值类型|返回值解释|
|:-----------|:---------- |:-----------|
{{ range .Returns }}| {{ .NameVerbose }} | %v{{ .TypeVerbose }}%v |  {{ .Description }} |
{{ end }}{{ end }}

{{ if eq (len .Example) 0  }} {{else}}

%v%v%vgo
{{ .Example }}
%v%v%v

{{end}}{{end}}
{{end}}

`,
	"`", "`", "`", "`", "`", "`", "`", "`", "`", "`", "`", "`", "`", "`")

func (l *LibDoc) ToMarkdown() string {
	var buffer = bytes.NewBuffer(nil)
	templateIns, err := template.New("yaklib-doc").Parse(yaklibMarkdownAPIDoc)
	if err != nil {
		log.Error(err)
		return ""
	}
	err = templateIns.Execute(buffer, l)
	if err != nil {
		log.Error(err)
		return ""
	}
	return buffer.String()
}
