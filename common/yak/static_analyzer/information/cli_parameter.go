package information

import "github.com/yaklang/yaklang/common/yak/ssaapi"

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
	SelectOption   map[string]string
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
		SelectOption:   nil,
	}
}

func ParseCliParameter(prog *ssaapi.Program) []*CliParameter {
	// prog.Show()
	ret := make([]*CliParameter, 0)

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

	handleOption := func(cli *CliParameter, opt *ssaapi.Value) {
		// opt.ShowUseDefChain()
		if !opt.IsCall() {
			// skip no function call
			return
		}
		arg1 := getConstString(opt.GetOperand(1))
		arg2 := getConstString(opt.GetOperand(2))

		// check option function, get information
		switch opt.GetOperand(0).String() {
		case "cli.setHelp":
			cli.Help = arg1
		case "cli.setRequired":
			cli.Required = getConstBool(opt.GetOperand(1))
		case "cli.setDefault":
			cli.Default = opt.GetOperand(1).GetConstValue()
		case "cli.setCliGroup":
			cli.Group = arg1
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
			if cli.SelectOption == nil {
				cli.SelectOption = make(map[string]string)
			}
			cli.SelectOption[arg1] = arg2
		}
	}

	parseCliFunction := func(funName, typ, methodTyp string) {
		prog.Ref(funName).GetUsers().Filter(
			func(v *ssaapi.Value) bool {
				// only function call and must be reachable
				return v.IsCall() && v.IsReachable() != -1
			},
		).ForEach(func(v *ssaapi.Value) {
			// cli.String("arg1", opt...)
			// op(0) => cli.String
			// op(1) => "arg1"
			// op(2...) => opt
			name := v.GetOperand(1).String()
			if v.GetOperand(1).IsConstInst() {
				name = v.GetOperand(1).GetConstValue().(string)
			}
			cli := newCliParameter(name, typ, methodTyp)
			opLen := len(v.GetOperands())
			// handler option
			for i := 2; i < opLen; i++ {
				handleOption(cli, v.GetOperand(i))
			}
			ret = append(ret, cli)
		})
	}

	for name, pair := range methodMap {
		parseCliFunction(name, pair.typ, pair.methodTyp)
	}

	return ret
}

type pair struct {
	typ       string
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
	}
	methodType2Method = map[string]string{
		"string":       "cli.String",
		"boolean":      "cli.Bool",
		"uint":         "cli.Int",
		"float":        "cli.Float",
		"file":         "cli.File",
		"file_names":   "cli.FileNames",
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
