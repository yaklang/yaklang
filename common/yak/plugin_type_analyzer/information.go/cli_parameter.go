package information

import "github.com/yaklang/yaklang/common/yak/ssaapi"

type CliParameter struct {
	Name           string
	NameVerbose    string
	Required       bool
	Type           string
	Default        any
	Help           string
	Group          string
	MultipleSelect bool
	SelectOption   map[string]string
}

func newCliParameter(typ, name string) *CliParameter {
	return &CliParameter{
		Name:           name,
		NameVerbose:    name,
		Required:       false,
		Type:           typ,
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

	parseCliFunction := func(funName, typName string) {
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
			cli := newCliParameter(typName, name)
			opLen := len(v.GetOperands())
			// handler option
			for i := 2; i < opLen; i++ {
				handleOption(cli, v.GetOperand(i))
			}
			ret = append(ret, cli)
		})
	}

	// second parameter is front-end render type, not Cli.Parameter type
	parseCliFunction("cli.String", "string")
	parseCliFunction("cli.Bool", "boolean") // "bool"
	parseCliFunction("cli.Int", "uint")     // "int"
	parseCliFunction("cli.Integer", "uint") // "int"
	parseCliFunction("cli.Double", "float")
	parseCliFunction("cli.Float", "float")

	parseCliFunction("cli.File", "upload-path")             // "file"
	parseCliFunction("cli.FileNames", "multiple-file-path") // "file-name" 多选文件路径
	parseCliFunction("cli.StringSlice", "select")           // "string-slice"
	parseCliFunction("cli.YakCode", "yak")
	parseCliFunction("cli.HTTPPacket", "http-packet")
	parseCliFunction("cli.Text", "text")

	parseCliFunction("cli.Url", "text") // 多行输入
	parseCliFunction("cli.Urls", "text")
	parseCliFunction("cli.Port", "string") // 单行输入
	parseCliFunction("cli.Ports", "string")
	parseCliFunction("cli.Net", "text")
	parseCliFunction("cli.Network", "text")
	parseCliFunction("cli.Host", "text")
	parseCliFunction("cli.Hosts", "text")
	parseCliFunction("cli.FileOrContent", "upload-file-content") // 单选文件并获取内容
	parseCliFunction("cli.LineDict", "upload-file-content")

	return ret
}
