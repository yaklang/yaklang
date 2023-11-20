package plugin_type_analyzer

import (
	"reflect"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type yakAnalyzer struct{}

var _ PluginTypeAnalyzer = (*yakAnalyzer)(nil)

func (y *yakAnalyzer) GetTypeSSAOpt() []ssaapi.Option {
	opts := make([]ssaapi.Option, 0)
	// yak function table
	symbol := yaklang.New().GetFntable()
	valueTable := make(map[string]interface{})
	// libTable := make(map[string]interface{})
	tmp := reflect.TypeOf(make(map[string]interface{}))
	for name, item := range symbol {
		itype := reflect.TypeOf(item)
		if itype == tmp {
			opts = append(opts, ssaapi.WithExternLib(name, item.(map[string]interface{})))
		} else {
			valueTable[name] = item
		}
	}

	// yak-main
	valueTable["YAK_DIR"] = ""
	valueTable["YAK_FILENAME"] = ""
	valueTable["YAK_MAIN"] = false
	valueTable["id"] = ""
	// param
	getParam := func(key string) interface{} {
		return nil
	}
	valueTable["getParam"] = getParam
	valueTable["getParams"] = getParam
	valueTable["param"] = getParam

	opts = append(opts, ssaapi.WithExternValue(valueTable))
	opts = append(opts, ssaapi.WithExternMethod(&builder{}))
	return opts
}

func (y *yakAnalyzer) CheckRule(prog *ssaapi.Program) {
}

func (y *yakAnalyzer) GetTypeInfo(prog *ssaapi.Program) []*YaklangInfo {
	ret := make([]*YaklangInfo, 0)

	cliList := ParseCliParameter(prog)
	cliInfo := NewYakLangInfo("cli")
	for _, cli := range cliList {
		cliInfo.AddKV(cli.ToInformation())
	}
	ret = append(ret, cliInfo)

	riskInfos := ParseRiskInfo(prog)
	riskInfo := NewYakLangInfo("risk")
	for _, risk := range riskInfos {
		riskInfo.AddKV(risk.ToInformation())
	}
	ret = append(ret, riskInfo)

	return ret
}

type CliParameter struct {
	Name     string
	Type     string
	Help     string
	Required bool
	Default  any
}

func newCliParameter(typ, name string) *CliParameter {
	return &CliParameter{
		Name:     name,
		Type:     typ,
		Help:     "",
		Required: false,
		Default:  nil,
	}
}

func (c *CliParameter) ToInformation() *YaklangInfoKV {
	ret := NewYaklangInfoKV("Name", c.Name)
	ret.AddExtern("Type", c.Type)
	ret.AddExtern("Help", c.Help)
	ret.AddExtern("Required", c.Required)
	ret.AddExtern("Default", c.Default)
	return ret
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

	handleOption := func(cli *CliParameter, opt *ssaapi.Value) {
		// opt.ShowUseDefChain()
		if !opt.IsCall() {
			// skip no function call
			return
		}

		// check option function, get information
		switch opt.GetOperand(0).String() {
		case "cli.setHelp":
			cli.Help = getConstString(opt.GetOperand(1))
		case "cli.setRequired":
			cli.Required = getConstString(opt.GetOperand(1)) == "true"
		case "cli.setDefault":
			cli.Default = opt.GetOperand(1).GetConstValue()
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

	parseCliFunction("cli.String", "string")
	parseCliFunction("cli.Bool", "bool")
	parseCliFunction("cli.Int", "int")
	parseCliFunction("cli.Integer", "int")
	parseCliFunction("cli.Double", "float")
	parseCliFunction("cli.Float", "float")
	parseCliFunction("cli.Url", "urls")
	parseCliFunction("cli.Urls", "urls")
	parseCliFunction("cli.Port", "port")
	parseCliFunction("cli.Ports", "port")
	parseCliFunction("cli.Net", "hosts")
	parseCliFunction("cli.Network", "hosts")
	parseCliFunction("cli.Host", "hosts")
	parseCliFunction("cli.Hosts", "hosts")
	parseCliFunction("cli.File", "file")
	parseCliFunction("cli.FileOrContent", "file_or_content")
	parseCliFunction("cli.LineDict", "file-or-content")
	parseCliFunction("cli.YakitPlugin", "yakit-plugin")
	parseCliFunction("cli.StringSlice", "string-slice")

	return ret
}

type RiskInfo struct {
	Level             string
	CVE               string
	Type, TypeVerbose string
}

func newRiskInfo() *RiskInfo {
	return &RiskInfo{
		Level:       "",
		CVE:         "",
		Type:        "",
		TypeVerbose: "",
	}
}

func (r *RiskInfo) ToInformation() *YaklangInfoKV {
	ret := NewYaklangInfoKV("Name", "risk")
	ret.AddExtern("Level", r.Level)
	ret.AddExtern("CVE", r.CVE)
	ret.AddExtern("Type", r.Type)
	ret.AddExtern("TypeVerbose", r.TypeVerbose)
	return ret
}

func ParseRiskInfo(prog *ssaapi.Program) []*RiskInfo {
	ret := make([]*RiskInfo, 0)
	getConstString := func(v *ssaapi.Value) string {
		if v.IsConstInst() {
			if str, ok := v.GetConstValue().(string); ok {
				return str
			}
		}
		//TODO: handler value with other opcode
		return ""
	}

	handleRiskLevel := func(level string) string {
		switch level {
		case "high":
			return "high"
		case "critical", "panic", "fatal":
			return "critical"
		case "warning", "warn", "middle", "medium":
			return "warning"
		case "info", "debug", "trace", "fingerprint", "note", "fp":
			return "info"
		case "low", "default":
			return "low"
		default:
			return "low"
		}
	}

	handleOption := func(riskInfo *RiskInfo, call *ssaapi.Value) {
		if !call.IsCall() {
			return
		}
		switch call.GetOperand(0).String() {
		case "risk.severity", "risk.level":
			riskInfo.Level = handleRiskLevel(getConstString(call.GetOperand(1)))
		case "risk.cve":
			riskInfo.CVE = call.GetOperand(1).String()
		case "risk.type":
			riskInfo.Type = getConstString(call.GetOperand(1))
			riskInfo.TypeVerbose = yakit.RiskTypeToVerbose(riskInfo.Type)
		case "risk.typeVerbose":
			riskInfo.TypeVerbose = getConstString(call.GetOperand(1))
		}
	}

	parseRiskFunction := func(name string, OptIndex int) {
		prog.Ref(name).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			riskInfo := newRiskInfo()
			optLen := len(v.GetOperands())
			for i := OptIndex; i < optLen; i++ {
				handleOption(riskInfo, v.GetOperand(i))
			}
			ret = append(ret, riskInfo)
		})
	}

	parseRiskFunction("risk.CreateRisk", 1)
	parseRiskFunction("risk.NewRisk", 1)

	return ret
}

// for method builder
type builder struct{}

var _ (ssa.MethodBuilder) = (*builder)(nil)

func (b *builder) Build(t ssa.Type, s string) *ssa.FunctionType {
	var (
		arg          = []ssa.Type{t}
		ret          = []ssa.Type{}
		IsVariadic   = false
		IsModifySelf = false
		name         = ""
	)

	var (
		StrTyp      = ssa.BasicTypes[ssa.String]
		NumberTyp   = ssa.BasicTypes[ssa.Number]
		BoolTyp     = ssa.BasicTypes[ssa.Boolean]
		AnyTyp      = ssa.BasicTypes[ssa.Any]
		BytesTyp    = ssa.BasicTypes[ssa.Bytes]
		HandlerFunc = func(arg, ret []ssa.Type, isVar bool) ssa.Type {
			return ssa.NewFunctionTypeDefine("handler", arg, ret, isVar)
		}
		SliceTyp = func(t ssa.Type) ssa.Type {
			return ssa.NewSliceType(t)
		}
		MapTyp = func(k ssa.Type, v ssa.Type) ssa.Type {
			return ssa.NewMapType(k, v)
		}
	)

	switch t.GetTypeKind() {
	case ssa.MapTypeKind:
		ot, _ := ssa.ToObjectType(t)
		fieldTyp := ot.FieldType
		keyTyp := ot.KeyTyp
		name += "map." + s
		switch s {
		case "Keys":
			ret = append(ret, SliceTyp(keyTyp))
		case "Values":
			ret = append(ret, SliceTyp(fieldTyp))
		case "Entries", "Item":
			//TODO: handle this return type: the map[T]U return [][1:T, 2:U]
			ret = append(ret, SliceTyp(AnyTyp))
		case "ForEach":
			arg = append(arg, HandlerFunc([]ssa.Type{keyTyp, fieldTyp}, []ssa.Type{}, false))
		case "Set":
			IsModifySelf = true
			arg = append(arg, keyTyp, fieldTyp)
			//TODO: this return value always True
			ret = append(ret, BoolTyp)
		case "Remove", "Delete":
			IsModifySelf = true
			arg = append(arg, keyTyp)
		case "Has", "IsExisted":
			arg = append(arg, keyTyp)
			ret = append(ret, BoolTyp)
		case "Length", "Len":
			ret = append(ret, NumberTyp)
		default:
			name = ""
		}
	case ssa.SliceTypeKind:
		ot, _ := ssa.ToObjectType(t)
		fieldTyp := ot.FieldType
		name = "slice." + s
		switch s {
		case "Append", "Push":
			IsModifySelf = true
			arg = append(arg, fieldTyp)
			ret = append(ret, ot)
		case "Pop":
			IsModifySelf = true
			ret = append(ret, fieldTyp)
		case "Extend", "Merge":
			IsModifySelf = true
			arg = append(arg, ot)
			ret = append(ret, ot)
		case "Length", "Len":
			ret = append(ret, NumberTyp)
		case "Capability", "Cap":
			ret = append(ret, NumberTyp)
		case "StringSlice":
			ret = append(ret, SliceTyp(StrTyp))
		case "GeneralSlice":
			ret = append(ret, SliceTyp(AnyTyp))
		case "Shift":
			IsModifySelf = true
			ret = append(ret, fieldTyp)
		case "Unshift":
			IsModifySelf = true
			arg = append(arg, fieldTyp)
			ret = append(ret, ot)
		case "Map":
			arg = append(arg, HandlerFunc([]ssa.Type{fieldTyp}, []ssa.Type{AnyTyp}, false))
			ret = append(ret, SliceTyp(AnyTyp))
		case "Filter":
			arg = append(arg, HandlerFunc([]ssa.Type{fieldTyp}, []ssa.Type{BoolTyp}, false))
			ret = append(ret, ot)
		case "Insert":
			IsModifySelf = true
			arg = append(arg, fieldTyp)
			ret = append(ret, ot)
		case "Remove":
			IsModifySelf = true
			arg = append(arg, fieldTyp)
			ret = append(ret, ot)
		case "Reverse":
			IsModifySelf = true
			ret = append(ret, ot)
		case "Sort":
			IsModifySelf = true
			arg = append(arg, BoolTyp)
			ret = append(ret, ot)
		case "Clear":
			IsModifySelf = true
			ret = append(ret, ot)
		case "Count":
			arg = append(arg, fieldTyp)
			ret = append(ret, NumberTyp)
		case "Index":
			arg = append(arg, NumberTyp)
			ret = append(ret, fieldTyp)
		default:
			name = ""
		}
	case ssa.String:
		name += "string." + s
		switch s {
		case "First":
			ret = append(ret, NumberTyp)
		case "Reverse":
			ret = append(ret, StrTyp)
		case "Shuffle":
			ret = append(ret, StrTyp)
		case "Fuzz":
			arg = append(arg, MapTyp(StrTyp, AnyTyp))
			ret = append(ret, SliceTyp(StrTyp))
			IsVariadic = true
		case "Contains":
			arg = append(arg, StrTyp)
			ret = append(ret, BoolTyp)
		case "IContains":
			arg = append(arg, StrTyp)
			ret = append(ret, BoolTyp)
		case "ReplaceN":
			arg = append(arg, StrTyp, StrTyp, NumberTyp)
			ret = append(ret, StrTyp)
		case "ReplaceAll", "Replace":
			arg = append(arg, StrTyp, StrTyp)
			ret = append(ret, StrTyp)
		case "Split":
			arg = append(arg, StrTyp)
			ret = append(ret, SliceTyp(StrTyp))
		case "SplitN":
			arg = append(arg, StrTyp, NumberTyp)
			ret = append(ret, SliceTyp(StrTyp))
		case "Join":
			arg = append(arg, SliceTyp(AnyTyp))
			ret = append(ret, StrTyp)
		case "Trim", "TrimLeft", "TrimRight":
			arg = append(arg, StrTyp)
			ret = append(ret, StrTyp)
			IsVariadic = true
		case "HasPrefix", "HasSuffix", "StartsWith", "EndsWith":
			arg = append(arg, StrTyp)
			ret = append(ret, BoolTyp)
		case "RemovePrefix", "RemoveSuffix":
			arg = append(arg, StrTyp)
			ret = append(ret, StrTyp)
		case "Zfill", "Rfill", "Lfill":
			arg = append(arg, NumberTyp)
			ret = append(ret, StrTyp)
		case "Ljust", "Rjust":
			arg = append(arg, NumberTyp, StrTyp)
			ret = append(ret, StrTyp)
			IsVariadic = true
		case "Count", "Find", "RFind", "IndexOf", "LastIndexOf":
			arg = append(arg, StrTyp)
			ret = append(ret, NumberTyp)
		case "Lower", "Upper", "Title":
			ret = append(ret, StrTyp)
		case "IsLower", "IsUpper", "IsTitle", "IsAlpha", "IsDigit", "IsAlnum", "IsPrintable":
			ret = append(ret, BoolTyp)
		default:
			name = ""
		}
	case ssa.Bytes:
		name = "bytes." + s
		switch s {
		case "First":
			ret = append(ret, NumberTyp)
		case "Reverse", "Shuffle":
			ret = append(ret, BytesTyp)
		case "FUzz":
			arg = append(arg, MapTyp(StrTyp, AnyTyp))
			ret = append(ret, SliceTyp(StrTyp))
			IsVariadic = true
		case "Contains", "IContains":
			arg = append(arg, BytesTyp)
			ret = append(ret, BoolTyp)
		case "ReplaceN":
			arg = append(arg, BytesTyp, BytesTyp, NumberTyp)
			ret = append(ret, BytesTyp)
		case "ReplaceAll", "Replace":
			arg = append(arg, BytesTyp, BytesTyp)
			ret = append(ret, BytesTyp)
		case "Split":
			arg = append(arg, BytesTyp)
			ret = append(ret, SliceTyp(BytesTyp))
		case "SplitN":
			arg = append(arg, BytesTyp, NumberTyp)
			ret = append(ret, SliceTyp(BytesTyp))
		case "Join":
			arg = append(arg, AnyTyp)
			ret = append(ret, BytesTyp)
		case "Trim", "TrimLeft", "TrimRight":
			arg = append(arg, BytesTyp)
			ret = append(ret, BytesTyp)
			IsVariadic = true
		case "HasPrefix", "HasSuffix", "StartsWith", "EndsWith":
			arg = append(arg, BytesTyp)
			ret = append(ret, BoolTyp)
		case "RemovePrefix", "RemoveSuffix":
			arg = append(arg, BytesTyp)
			ret = append(ret, BytesTyp)
		case "Zfill", "Rzfill":
			arg = append(arg, NumberTyp)
			ret = append(ret, BytesTyp)
		case "Ljust", "Rjust":
			arg = append(arg, NumberTyp, BytesTyp)
			ret = append(ret, BytesTyp)
			IsVariadic = true
		case "Count", "Find", "Rfind", "IndexOf", "LastIndexOf":
			arg = append(arg, BytesTyp)
			ret = append(ret, NumberTyp)
		case "Lower", "Upper", "Title":
			ret = append(ret, BytesTyp)
		case "IsLower", "IsUpper", "IsTitle", "IsAlpha", "IsDigit", "IsAlnum", "IsPrintable":
			ret = append(ret, BoolTyp)
		default:
			name = ""
		}
	}
	if name != "" {
		f := ssa.NewFunctionTypeDefine(name, arg, ret, IsVariadic)
		f.SetModifySelf(IsModifySelf)
		return f
	} else {
		return nil
	}
}

func (b *builder) GetMethodNames(t ssa.Type) []string {
	switch t.GetTypeKind() {
	case ssa.SliceTypeKind:
		return []string{
			"Append", "Push", "Pop", "Extend", "Merge", "Length", "Len", "Capability", "Cap", "StringSlice", "GeneralSlice", "Shift", "Unshift", "Map", "Filter", "Insert", "Remove", "Reverse", "Sort", "Clear", "Count", "Index",
		}
	case ssa.String:
		return []string{
			"Reverse", "Shuffle", "Fuzz", "Contains", "IContains", "ReplaceN", "ReplaceAll", "Replace", "Split", "SplitN", "Join", "Trim", "TrimLeft", "TrimRight", "HasPrefix", "HasSuffix", "StartsWith", "EndsWith", "RemovePrefix", "RemoveSuffix", "Zfill", "Rfill", "Lfill", "Ljust", "Rjust", "Count", "Find", "RFind", "IndexOf", "LastIndexOf", "Lower", "Upper", "Title", "IsLower", "IsUpper", "IsTitle", "IsAlpha", "IsDigit", "IsAlnum", "IsPrintable",
		}
	case ssa.MapTypeKind:
		return []string{
			"Keys", "Values", "Entries", "Item", "ForEach", "Set", "Remove", "Delete", "Has", "IsExisted", "Length", "Len",
		}
	case ssa.Bytes:
		return []string{
			"First", "Reverse", "Shuffle", "FUzz", "Contains", "IContains", "ReplaceN", "ReplaceAll", "Replace", "Split", "SplitN", "Join", "Trim", "TrimLeft", "TrimRight", "HasPrefix", "HasSuffix", "StartsWith", "EndsWith", "RemovePrefix", "RemoveSuffix", "Zfill", "Rzfill", "Ljust", "Rjust", "Count", "Find", "Rfind", "IndexOf", "LastIndexOf", "Lower", "Upper", "Title", "IsLower", "IsUpper", "IsTitle", "IsAlpha", "IsDigit", "IsAlnum", "IsPrintable",
		}
	default:
		return []string{}
	}
}
