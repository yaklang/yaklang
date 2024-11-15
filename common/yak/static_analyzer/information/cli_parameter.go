package information

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type CliParameter struct {
	Name           string
	NameVerbose    string
	Required       bool
	Type           string
	MethodType     string
	Default        any
	Help           string
	Group          string
	MultipleSelect bool
	SelectOption   *orderedmap.OrderedMap
	JsonSchema     string
}

type UIInfo struct {
	Typ            string   // "show", "hide", "..."
	Effected       []string // parameter names
	effectGroup    []string // group names, will turn to `effected`
	WhenExpression string   // when expression
}

func newCliParameter(name, typ, methodTyp string) *CliParameter {
	return &CliParameter{
		Name:           name,
		NameVerbose:    name,
		Required:       false,
		Type:           typ,
		MethodType:     methodTyp,
		Default:        nil,
		Help:           "",
		Group:          "",
		MultipleSelect: false,
		SelectOption:   orderedmap.New(),
	}
}

func ParseCliParameter(prog *ssaapi.Program) ([]*CliParameter, []*UIInfo) {
	// prog.Show()
	params := make([]*CliParameter, 0)
	uiInfos := make([]*UIInfo, 0)
	groups := make(map[string][]*CliParameter)

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

	handleOption := func(cli *CliParameter, opt *ssaapi.Value) {
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
		}
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
			for i := 2; i < opLen; i++ {
				handleOption(cli, v.GetOperand(i))
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

	return params, uiInfos
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
