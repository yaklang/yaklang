package information

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type CliParameter struct {
	Name                      string
	NameVerbose               string
	Required                  bool
	Type                      string
	MethodType                string
	Default                   any
	Help                      string
	Group                     string
	SuggestionValueExpression string
	MultipleSelect            bool
	SelectOption              *orderedmap.OrderedMap
	JsonSchema                string
	UISchema                  string
}

type UIInfo struct {
	Typ            string   // "show", "hide", "..."
	Effected       []string // parameter names
	effectGroup    []string // group names, will turn to `effected`
	WhenExpression string   // when expression
}

type UISchemaInfo struct {
	Grid                 *UISchemaGrid
	TablePageWidth       int
	GlobalFieldClassName cli.UISchemaFieldClassName
}

type UISchemaGrid struct {
	Groups []*UISchemaGroup
}

type UISchemaGroup struct {
	Fields []*UISchemaField
}

type UISchemaField struct {
	FieldName      string
	Width          int
	ClassName      cli.UISchemaFieldClassName
	Widget         cli.UISchemaWidgetType
	ComponentStyle map[string]any
	InnerGroups    []*UISchemaGroup
}

func (info *UISchemaInfo) ToUISchema() (string, error) {

	globalMap := orderedmap.New()
	addToOrderedMap := func(tmp *orderedmap.OrderedMap, field, k string, v any) *orderedmap.OrderedMap {
		if _, ok := tmp.Get(field); !ok {
			tmp.Set(field, orderedmap.New())
		}
		i, _ := tmp.Get(field)
		m := i.(*orderedmap.OrderedMap)
		m.Set(k, v)
		return m
	}

	var (
		handleGroups func(innerMap *orderedmap.OrderedMap, groups []*UISchemaGroup) []*orderedmap.OrderedMap
		handleField  func(gridMap, innerMap *orderedmap.OrderedMap, field *UISchemaField)
	)

	handleGroups = func(innerMap *orderedmap.OrderedMap, groups []*UISchemaGroup) []*orderedmap.OrderedMap {
		gridMaps := make([]*orderedmap.OrderedMap, 0)
		for _, group := range groups {
			gridMap := orderedmap.New()
			for _, field := range group.Fields {
				handleField(gridMap, innerMap, field)
			}
			gridMaps = append(gridMaps, gridMap)
		}
		return gridMaps
	}

	handleField = func(gridMap, innerMap *orderedmap.OrderedMap, field *UISchemaField) {
		if field == nil {
			return
		}
		gridMap.Set(field.FieldName, field.Width)
		if field.ClassName != cli.UISchemaFieldPosDefault {
			addToOrderedMap(innerMap, field.FieldName, "ui:classNames", string(field.ClassName))
		}
		if len(field.ComponentStyle) > 0 {
			addToOrderedMap(innerMap, field.FieldName, "ui:component_style", field.ComponentStyle)
		}
		if field.Widget != cli.UISchemaWidgetDefault {
			addToOrderedMap(innerMap, field.FieldName, "ui:widget", string(field.Widget))
		}
		if len(field.InnerGroups) > 0 {
			newInnerMap := orderedmap.New()
			gridMaps := handleGroups(newInnerMap, field.InnerGroups)
			newInnerMap.ForEach(func(key string, value any) {
				addToOrderedMap(innerMap, field.FieldName, key, value)
			})
			addToOrderedMap(innerMap, field.FieldName, "ui:grid", gridMaps)
		}
	}

	if grid := info.Grid; grid != nil {
		globalMap.Set("ui:grid", handleGroups(globalMap, grid.Groups))
	}
	if info.GlobalFieldClassName != "" {
		globalMap.Set("ui:classNames", string(info.GlobalFieldClassName))
	}
	if info.TablePageWidth != 0 {
		globalMap.Set("x", info.TablePageWidth)
	}

	bytes, err := json.Marshal(globalMap)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func newCliParameter(name, typ, methodTyp string) *CliParameter {
	return &CliParameter{
		Name:                      name,
		NameVerbose:               name,
		Required:                  false,
		Type:                      typ,
		MethodType:                methodTyp,
		Default:                   nil,
		Help:                      "",
		Group:                     "",
		MultipleSelect:            false,
		SuggestionValueExpression: "",
		SelectOption:              orderedmap.New(),
	}
}

func ParseCliParameter(prog *ssaapi.Program) ([]*CliParameter, []*UIInfo, []string) {
	if prog == nil {
		return nil, nil, nil
	}
	// prog.Show()
	params := make([]*CliParameter, 0)
	uiInfos := make([]*UIInfo, 0)
	groups := make(map[string][]*CliParameter)
	envKeys := make([]string, 0)

	getConstString := func(v *ssaapi.Value) string {
		if str, ok := v.GetConstValue().(string); ok {
			return str
		}
		return ""
	}
	getConstBool := func(v *ssaapi.Value) bool {
		if b, ok := v.GetConstValue().(bool); ok {
			return b
		}
		return false
	}

	handleUIOption := func(ui *UIInfo, opt *ssaapi.Value) {
		// opt.ShowUseDefChain()
		if !opt.IsCall() {
			// skip no function call
			return
		}
		// check option function, get information
		funcName := opt.GetOperand(0).GetName()
		if strings.HasPrefix(funcName, "cli.show") {
			ui.Typ = "show"
		} else if strings.HasPrefix(funcName, "cli.hide") {
			ui.Typ = "hide"
		}

		switch funcName {
		case "cli.showGroup", "cli.hideGroup":
			ui.effectGroup = append(ui.effectGroup, getConstString(opt.GetOperand(1)))
		case "cli.showParams", "cli.hideParams":
			operands := opt.GetOperands()
			if len(operands) < 2 {
				break
			} else {
				operands = operands[1:] // skip self
			}

			if ui.Effected == nil {
				ui.Effected = make([]string, 0, len(operands))
			}
			operands.ForEach(func(v *ssaapi.Value) {
				ui.Effected = append(ui.Effected, getConstString(v))
			})
		case "cli.when":
			ui.WhenExpression = getConstString(opt.GetOperand(1))
		case "cli.whenTrue":
			ui.WhenExpression = fmt.Sprintf("%s == true", getConstString(opt.GetOperand(1)))
		case "cli.whenFalse":
			ui.WhenExpression = fmt.Sprintf("%s == false", getConstString(opt.GetOperand(1)))
		case "cli.whenEqual":
			ui.WhenExpression = fmt.Sprintf("%s == %s", getConstString(opt.GetOperand(1)), getConstString(opt.GetOperand(2)))
		case "cli.whenNotEqual":
			ui.WhenExpression = fmt.Sprintf("%s != %s", getConstString(opt.GetOperand(1)), getConstString(opt.GetOperand(2)))
		case "cli.whenDefault":
			ui.WhenExpression = "true"
		}
	}

	handleUIFieldPosition := func(s string) cli.UISchemaFieldClassName {
		switch s {
		case "cli.uiPosDefault":
			return cli.UISchemaFieldPosDefault
		case "cli.uiPosHorizontal":
			return cli.UISchemaFieldPosHorizontal
		default:
			return cli.UISchemaFieldPosDefault
		}
	}

	handleUIFieldWidget := func(s string) cli.UISchemaWidgetType {
		switch s {
		case "cli.uiWidgetTable":
			return cli.UISchemaWidgetTable
		case "cli.uiWidgetRadio":
			return cli.UISchemaWidgetRadio
		case "cli.uiWidgetSelect":
			return cli.UISchemaWidgetSelect
		case "cli.uiWidgetCheckbox":
			return cli.UISchemaWidgetCheckbox
		case "cli.uiWidgetTextarea":
			return cli.UISchemaWidgetTextArea
		case "cli.uiWidgetPassword":
			return cli.UISchemaWidgetPassword
		case "cli.uiWidgetColor":
			return cli.UISchemaWidgetColor
		case "cli.uiWidgetEmail":
			return cli.UISchemaWidgetEmail
		case "cli.uiWidgetUri":
			return cli.UISchemaWidgetUri
		case "cli.uiWidgetDate":
			return cli.UISchemaWidgetDate
		case "cli.uiWidgetDateTime":
			return cli.UISchemaWidgetDateTime
		case "cli.uiWidgetTime":
			return cli.UISchemaWidgetTime
		case "cli.uiWidgetUpdown":
			return cli.UISchemaWidgetUpdown
		case "cli.uiWidgetRange":
			return cli.UISchemaWidgetRange
		case "cli.uiWidgetFile":
			return cli.UISchemaWidgetFile
		case "cli.uiWidgetFiles":
			return cli.UISchemaWidgetFiles
		case "cli.uiWidgetFolder":
			return cli.UISchemaWidgetFolder
		default:
			return cli.UISchemaWidgetDefault
		}
	}

	handleUIFieldMap := func(value *ssaapi.Value) map[string]any {
		res := make(map[string]any)
		members := value.GetMembers()
		for _, item := range members {
			if len(item) < 2 {
				continue
			}
			key, value := item[0], item[1]
			keyConst := key.GetConst()
			if keyConst == nil {
				continue
			}
			res[keyConst.String()] = value.GetConstValue()
		}
		return res
	}

	var (
		handleUISchemaGroup func(opt *ssaapi.Value) *UISchemaGroup
		handleUISchemaField func(opt *ssaapi.Value) *UISchemaField
	)

	handleUISchemaField = func(opt *ssaapi.Value) *UISchemaField {
		fieldArgs := opt.GetOperands()
		if len(fieldArgs) == 0 {
			return nil
		}
		isTableField := false
		switch fieldArgs[0].GetName() {
		case "cli.uiTableField":
			isTableField = true
		case "cli.uiField":
		default:
			return nil
		}
		field := new(UISchemaField)
		field.FieldName = getConstString(fieldArgs[1])
		secondFieldConst := fieldArgs[2].GetConst()
		if !isTableField {
			widthPercent := 1.0
			if secondFieldConst.IsFloat() {
				widthPercent = secondFieldConst.Float()
			} else if secondFieldConst.IsNumber() {
				widthPercent = float64(secondFieldConst.Number())
			} else {
				log.Errorf("field width is invalid: %v", secondFieldConst)
			}

			field.Width = int(math.Round(widthPercent * 24.0))
		} else {
			width := 0.0
			if secondFieldConst.IsFloat() {
				width = secondFieldConst.Float()
			} else if secondFieldConst.IsNumber() {
				width = float64(secondFieldConst.Number())
			} else {
				log.Errorf("field width is invalid: %v", secondFieldConst)
			}
			field.Width = int(width)
		}

		if len(fieldArgs) > 2 {
			for _, fieldArg := range fieldArgs[3:] {
				if !fieldArg.IsCall() {
					continue
				}
				fieldParamArg := fieldArg.GetOperands()
				if len(fieldParamArg) < 2 {
					continue
				}
				switch fieldParamArg[0].GetName() {
				case "cli.uiFieldPosition":
					field.ClassName = handleUIFieldPosition(fieldParamArg[1].GetName())
				case "cli.uiFieldWidget":
					field.Widget = handleUIFieldWidget(fieldParamArg[1].GetName())
				case "cli.uiFieldComponentStyle":
					field.ComponentStyle = handleUIFieldMap(fieldParamArg[1])
				case "cli.uiFieldGroups":
					for _, arg := range fieldParamArg[1:] {
						field.InnerGroups = append(field.InnerGroups, handleUISchemaGroup(arg))
					}
				}
			}
		}
		return field
	}

	handleUISchemaGroup = func(opt *ssaapi.Value) *UISchemaGroup {
		if !opt.IsCall() {
			// skip no function call
			return nil
		}
		args := opt.GetOperands()
		if len(args) == 0 {
			return nil
		}
		group := new(UISchemaGroup)
		for _, arg := range args[1:] {
			field := handleUISchemaField(arg)
			if field == nil {
				continue
			}
			group.Fields = append(group.Fields, field)
		}
		return group
	}

	handleUISchemaOption := func(ui *UISchemaInfo, opt *ssaapi.Value) {
		if !opt.IsCall() {
			// skip no function call
			return
		}
		args := opt.GetOperands()
		if len(args) == 0 {
			return
		}
		switch args[0].GetName() {
		case "cli.uiGlobalFieldPosition":
			ui.GlobalFieldClassName = handleUIFieldPosition(args[1].GetName())
		case "cli.uiGroups":
			if ui.Grid == nil {
				ui.Grid = new(UISchemaGrid)
			}
			for _, arg := range args[1:] {
				ui.Grid.Groups = append(ui.Grid.Groups, handleUISchemaGroup(arg))
			}
		}
	}

	handleUISchema := func(cli *CliParameter, opt *ssaapi.Value) {
		// opt.ShowUseDefChain()
		if !opt.IsCall() {
			// skip no function call
			return
		}
		// check option function, get information
		args := opt.GetOperands()
		if len(args) == 0 {
			return
		}
		switch args[0].GetName() {
		case "cli.setUISchema":
			if len(args) > 1 {
				info := new(UISchemaInfo)
				for _, arg := range args[1:] {
					handleUISchemaOption(info, arg)
				}
				if uiSchema, err := info.ToUISchema(); err != nil {
					log.Errorf("convert ui schema error: %v", err)
				} else {
					cli.UISchema = uiSchema
				}
			}
		}
	}

	handleOption := func(cli *CliParameter, opt *ssaapi.Value) (skip bool) {
		// opt.ShowUseDefChain()
		if !opt.IsCall() {
			// skip no function call
			return
		}
		arg1 := getConstString(opt.GetOperand(1))
		arg2 := getConstString(opt.GetOperand(2))

		// check option function, get information
		switch opt.GetOperand(0).GetName() {
		case "cli.setHelp":
			cli.Help = arg1
		case "cli.setRequired":
			cli.Required = getConstBool(opt.GetOperand(1))
		case "cli.setDefault":
			cli.Default = opt.GetOperand(1).GetConstValue()
		case "cli.setCliGroup":
			cli.Group = arg1
			if _, ok := groups[arg1]; !ok {
				groups[arg1] = make([]*CliParameter, 0)
			}
			groups[arg1] = append(groups[arg1], cli)

		case "cli.setVerboseName":
			cli.NameVerbose = arg1
		case "cli.setMultipleSelect": // only for `cli.StringSlice`
			if cli.Type != "select" {
				break
			}
			cli.MultipleSelect = getConstBool(opt.GetOperand(1))
		case "cli.setSelectOption": // only for `cli.StringSlice`
			if cli.Type != "select" {
				break
			}
			if arg1 == "" {
				cli.SelectOption.Set(arg2, arg2)
			} else {
				cli.SelectOption.Set(arg1, arg2)
			}
		case "cli.setJsonSchema":
			if cli.Type != "json" {
				break
			}
			cli.JsonSchema = arg1
			args := opt.GetOperands()
			if len(args) > 2 {
				handleUISchema(cli, args[2])
			}
		case "cli.setYakitPayload":
			if getConstBool(opt.GetOperand(1)) {
				cli.SuggestionValueExpression = "db.GetAllPayloadGroupsName()"
			}
		case "cli.setPluginEnv":
			key := ""
			if opt.GetOperand(1).IsConstInst() {
				key = opt.GetOperand(1).GetConst().VarString()
			} else {
				key = opt.GetOperand(1).String()
			}
			envKeys = append(envKeys, key)
			skip = true
		}
		return
	}
	parseUiFunc := func(v *ssaapi.Value) {
		v.GetUsers().Filter(
			func(v *ssaapi.Value) bool {
				// only function call and must be reachable
				return v.IsCall() && v.IsReachable() != -1
			},
		).ForEach(func(v *ssaapi.Value) {
			ui := new(UIInfo)
			uiInfos = append(uiInfos, ui)

			v.GetOperands().ForEach(func(v *ssaapi.Value) {
				handleUIOption(ui, v)
			})
		})
	}

	parseCliParameterFunc := func(v *ssaapi.Value, typ, methodTyp string) {
		v.GetUsers().Filter(
			func(v *ssaapi.Value) bool {
				// only function call and must be reachable
				return v.IsCall() && v.IsReachable() != -1
			},
		).ForEach(func(v *ssaapi.Value) {
			// cli.String("arg1", opt...)
			// op(0) => cli.String
			// op(1) => "arg1"
			// op(2...) => opt

			nameValue := v.GetOperand(1)
			name := ""
			if nameValue.IsConstInst() {
				if c := nameValue.GetConst(); c.IsString() {
					name = c.VarString()
				}
			} else {
				name = nameValue.String()
			}

			if name == "" {
				return
			}
			cli := newCliParameter(name, typ, methodTyp)
			opLen := len(v.GetOperands())
			// handler option
			shouldSkip := false
			for i := 2; i < opLen; i++ {
				if handleOption(cli, v.GetOperand(i)) {
					shouldSkip = true
				}
			}
			if shouldSkip {
				return
			}
			params = append(params, cli)
		})
	}

	prog.Ref("cli").GetOperands().ForEach(func(v *ssaapi.Value) {
		if !v.IsFunction() {
			return
		}
		// ui info
		funcName := v.GetName()
		if funcName == "cli.UI" {
			parseUiFunc(v)
			return
		}

		// cli parameter
		pair, ok := methodMap[funcName]
		if ok {
			parseCliParameterFunc(v, pair.typ, pair.methodTyp)
		}
	})

	// handle effect group
	for _, info := range uiInfos {
		for _, name := range info.effectGroup {
			group, ok := groups[name]
			if !ok {
				continue
			}
			for _, param := range group {
				info.Effected = append(info.Effected, param.Name)
			}
		}
	}

	return params, uiInfos, envKeys
}

type pair struct {
	// for frontend component select
	typ string // yakit/app/renderer/src/main/src/pages/plugins/operator/localPluginExecuteDetailHeard/LocalPluginExecuteDetailHeard.tsx
	// for code generation  in yaklang/common/yakgrpc/grpc_yaklang_inspect_information.go
	methodTyp string
}

var (
	methodMap = map[string]pair{
		"cli.String":        {"string", "string"},
		"cli.Bool":          {"boolean", "boolean"},
		"cli.Int":           {"uint", "uint"},
		"cli.Integer":       {"uint", "uint"},
		"cli.Double":        {"float", "float"},
		"cli.Float":         {"float", "float"},
		"cli.File":          {"upload-path", "file"},
		"cli.FileNames":     {"multiple-file-path", "file_names"},
		"cli.FolderName":    {"upload-folder-path", "folder_name"},
		"cli.StringSlice":   {"select", "select"},
		"cli.YakCode":       {"yak", "yak"},
		"cli.HTTPPacket":    {"http-packet", "http-packet"},
		"cli.Text":          {"text", "text"},
		"cli.Url":           {"text", "urls"},
		"cli.Urls":          {"text", "urls"},
		"cli.Port":          {"string", "ports"},
		"cli.Ports":         {"string", "ports"},
		"cli.Net":           {"text", "hosts"},
		"cli.Network":       {"text", "hosts"},
		"cli.Host":          {"text", "hosts"},
		"cli.Hosts":         {"text", "hosts"},
		"cli.FileOrContent": {"upload-file-content", "file_content"},
		"cli.LineDict":      {"upload-file-content", "line_dict"},
		"cli.Json":          {"json", "json"},
	}
	methodType2Method = map[string]string{
		"string":       "cli.String",
		"boolean":      "cli.Bool",
		"uint":         "cli.Int",
		"float":        "cli.Float",
		"file":         "cli.File",
		"file_names":   "cli.FileNames",
		"folder_name":  "cli.FolderName",
		"select":       "cli.StringSlice",
		"yak":          "cli.YakCode",
		"http-packet":  "cli.HTTPPacket",
		"text":         "cli.Text",
		"urls":         "cli.Urls",
		"ports":        "cli.Ports",
		"hosts":        "cli.Hosts",
		"file_content": "cli.FileOrContent",
		"line_dict":    "cli.LineDict",
	}
)

func CliParam2grpc(params []*CliParameter) []*ypb.YakScriptParam {
	ret := make([]*ypb.YakScriptParam, 0, len(params))

	for _, param := range params {
		defaultValue := ""
		if param.Default != nil {
			defaultValue = fmt.Sprintf("%v", param.Default)
		}
		extra := []byte{}
		if param.Type == "select" {
			paramSelect := &PluginParamSelect{
				Double: param.MultipleSelect,
				Data:   make([]PluginParamSelectData, 0),
			}
			param.SelectOption.ForEach(func(k string, v any) {
				paramSelect.Data = append(paramSelect.Data, PluginParamSelectData{
					Key:   k,
					Label: k,
					Value: codec.AnyToString(v),
				})
			})
			extra, _ = json.Marshal(paramSelect)
		}

		ret = append(ret, &ypb.YakScriptParam{
			Field:                    param.Name,
			DefaultValue:             string(defaultValue),
			TypeVerbose:              param.Type,
			FieldVerbose:             param.NameVerbose,
			Help:                     param.Help,
			Required:                 param.Required,
			Group:                    param.Group,
			SuggestionDataExpression: param.SuggestionValueExpression,
			ExtraSetting:             string(extra),
			MethodType:               param.MethodType,
			JsonSchema:               param.JsonSchema,
			UISchema:                 param.UISchema,
		})
	}

	return ret
}

func GenerateParameterFromProgram(prog *ssaapi.Program) (string, string, error) {
	parameters, _, pluginEnvKey := ParseCliParameter(prog) //
	cli := CliParam2grpc(parameters)

	getMarshalData := func(data interface{}) (string, error) {
		if funk.IsEmpty(data) {
			return "", nil
		}
		dataRaw, err := json.Marshal(data)
		if err != nil {
			return "", err
		}
		return strconv.Quote(string(dataRaw)), err
	}

	parameterRaw, err := getMarshalData(cli)
	if err != nil {
		return "", "", err
	}
	pluginEnvKeyRaw, err := getMarshalData(pluginEnvKey)
	if err != nil {
		return "", "", err
	}
	return parameterRaw, pluginEnvKeyRaw, nil
}

type PluginParamSelect struct {
	Double bool                    `json:"double"`
	Data   []PluginParamSelectData `json:"data"`
}

type PluginParamSelectData struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

func UiInfo2grpc(info []*UIInfo) []*ypb.YakUIInfo {
	ret := make([]*ypb.YakUIInfo, 0, len(info))
	for _, i := range info {
		ret = append(ret, &ypb.YakUIInfo{
			Typ:            i.Typ,
			Effected:       i.Effected,
			WhenExpression: i.WhenExpression,
		})
	}
	return ret
}

func RiskInfo2grpc(info []*RiskInfo, db *gorm.DB) []*ypb.YakRiskInfo {
	ret := make([]*ypb.YakRiskInfo, 0, len(info))
	for _, i := range info {
		description := i.Description
		solution := i.Solution

		if (description == "" || solution == "") && i.CVE != "" {
			if db != nil {
				cve, err := cveresources.GetCVE(db, i.CVE)
				if err == nil {
					if description == "" {
						description = cve.DescriptionMainZh
					}
					if solution == "" {
						solution = cve.Solution
					}
					if i.Level == "" {
						i.Level = cve.Severity
					}
				}
			}
		}

		ret = append(ret, &ypb.YakRiskInfo{
			Level:       i.Level,
			TypeVerbose: i.TypeVerbose,
			CVE:         i.CVE,
			Description: description,
			Solution:    solution,
		})
	}
	return ret
}
