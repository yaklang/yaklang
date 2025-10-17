package rules

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func init() {
	// cli
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeYak, RuleCliDefault)
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeYak, RuleCliParamName)
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeYak, RuleCliCheck)
}

// 检查 cli.setDefault 设置的默认值是否符合规范
func RuleCliDefault(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("cli-default type check")
	// tag := "SSA-cli-setDefault"
	checkCliDefault := func(funcName string, typs []*ssaapi.Type, checkCallBack func(funcName string, v *ssaapi.Value) (string, bool)) {
		prog.Ref(funcName).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			ops := v.GetOperands()

			for i := 2; i < len(ops); i++ {
				opt := ops[i]
				optFuncName := opt.GetOperand(0).String()
				if optFuncName != "cli.setDefault" {
					continue
				}
				field := opt.GetOperand(1)
				if field == nil {
					break
				}
				if !field.IsConstInst() {
					ret.NewWarn(fmt.Sprintf("%s want const value, but not", funcName), field)
					break
				}

				fieldTyp := field.GetType()
				pass := false
				for _, typ := range typs {
					if fieldTyp.Compare(typ) {
						pass = true
						break
					}
				}
				if !pass {
					ret.NewError(
						fmt.Sprintf("%s want [%s] type, but got [%s] type", funcName,
							strings.Join(lo.Map(typs, func(typ *ssaapi.Type, _ int) string { return typ.String() }), "|"),
							fieldTyp),
						field,
					)
					break
				}

				if checkCallBack != nil {
					message, ok := checkCallBack(funcName, field)
					if !ok {
						ret.NewError(message, field)
						break
					}
				}
			}
		})
	}
	urlsCallback := func(funcName string, v *ssaapi.Value) (string, bool) {
		// 如果不是constInst，无法分析
		if !v.IsConstInst() {
			return "", true
		}
		consts := v.GetConst()
		urls := utils.ParseStringToUrlsWith3W(utils.ParseStringToHosts(consts.String())...)
		if len(urls) == 0 {
			return fmt.Sprintf("%s want valid url, but got %s", funcName, consts.String()), false
		}
		for _, u := range urls {
			parsed, err := url.Parse(u)
			if err != nil {
				return fmt.Sprintf("%s want valid url, but got %#v", funcName, u), false
			}
			if parsed.Host == "" {
				return fmt.Sprintf("%s want valid url, but got %#v", funcName, u), false
			}
		}
		return "", true
	}

	portsCallback := func(funcName string, v *ssaapi.Value) (string, bool) {
		// 如果不是constInst，无法分析
		if !v.IsConstInst() {
			return "", true
		}
		consts := v.GetConst()
		ports := utils.ParseStringToPorts(consts.String())
		if len(ports) == 0 {
			return fmt.Sprintf("%s want valid port, but got %#v", funcName, consts.String()), false
		}
		sort.Ints(ports)
		if len(ports) > 0 {
			p := ports[0]
			if p <= 0 {
				return fmt.Sprintf("%s want valid port, but got %d", funcName, p), false
			}
			p = ports[len(ports)-1]
			if p > 65535 {
				return fmt.Sprintf("%s want valid port, but got %d", funcName, p), false
			}
		}

		return "", true
	}

	hostCallback := func(funcName string, v *ssaapi.Value) (string, bool) {
		// 如果不是constInst，无法分析
		if !v.IsConstInst() {
			return "", true
		}
		consts := v.GetConst()
		hosts := utils.ParseStringToHosts(consts.String())
		// fmt.Printf("debug : %#v\n", consts.String())
		if len(hosts) == 0 {
			return fmt.Sprintf("%s want valid hosts, but got %#v", funcName, consts.String()), false
		}
		return "", true
	}

	fileCallback := func(funcName string, v *ssaapi.Value) (string, bool) {
		// 如果不是constInst，无法分析
		if !v.IsConstInst() {
			return "", true
		}
		consts := v.GetConst()
		fileName := consts.String()
		if utils.GetFirstExistedPath(fileName) == "" {
			return fmt.Sprintf("filepath [%s] not existed", fileName), false
		}
		_, err := os.ReadFile(fileName)
		if err != nil {
			return fmt.Sprintf("filepath [%s] read error: %s", fileName, err.Error()), false
		}

		return "", true
	}

	fileOrContentCallback := func(funcName string, v *ssaapi.Value) (string, bool) {
		// 如果不是constInst，无法分析
		if !v.IsConstInst() {
			return "", true
		}
		consts := v.GetConst()
		raw := consts.String()
		if utils.StringAsFileParams(raw) == nil {
			return fmt.Sprintf("invalid filepath or empty content: [%s]", raw), false
		}

		return "", true
	}
	intCallback := func(funcName string, v *ssaapi.Value) (string, bool) {
		consts := v.GetConst()
		raw := consts.String()
		_, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Sprintf("%s want valid int, but got %s", funcName, raw), false
		}
		return "", true
	}
	floatCallback := func(funcName string, v *ssaapi.Value) (string, bool) {
		consts := v.GetConst()
		raw := consts.String()
		_, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Sprintf("%s want valid float, but got %s", funcName, raw), false
		}
		return "", true
	}

	checkCliDefault("cli.String", []*ssaapi.Type{String}, nil)
	checkCliDefault("cli.StringSlice", []*ssaapi.Type{String}, nil)
	checkCliDefault("cli.Bool", []*ssaapi.Type{Boolean}, nil)
	checkCliDefault("cli.Int", []*ssaapi.Type{Number, String}, intCallback)
	checkCliDefault("cli.Integer", []*ssaapi.Type{Number, String}, intCallback)
	checkCliDefault("cli.Double", []*ssaapi.Type{Number, String}, floatCallback) // 需要区分 int 和 double
	checkCliDefault("cli.Float", []*ssaapi.Type{Number, String}, floatCallback)  // 需要区分 int 和 double
	checkCliDefault("cli.Url", []*ssaapi.Type{Any}, urlsCallback)
	checkCliDefault("cli.Urls", []*ssaapi.Type{Any}, urlsCallback)
	checkCliDefault("cli.Port", []*ssaapi.Type{Any}, portsCallback)
	checkCliDefault("cli.Ports", []*ssaapi.Type{Any}, portsCallback)
	checkCliDefault("cli.Net", []*ssaapi.Type{Any}, hostCallback)
	checkCliDefault("cli.Network", []*ssaapi.Type{Any}, hostCallback)
	checkCliDefault("cli.Host", []*ssaapi.Type{Any}, hostCallback)
	checkCliDefault("cli.Hosts", []*ssaapi.Type{Any}, hostCallback)
	checkCliDefault("cli.File", []*ssaapi.Type{String}, fileCallback)
	checkCliDefault("cli.FileOrContent", []*ssaapi.Type{String}, fileOrContentCallback)
	checkCliDefault("cli.LineDict", []*ssaapi.Type{String}, fileOrContentCallback)
	checkCliDefault("cli.YakitPlugin", []*ssaapi.Type{String}, fileCallback)
	checkCliDefault("cli.Have", []*ssaapi.Type{String}, nil)
	checkCliDefault("cli.HTTPPacket", []*ssaapi.Type{String}, nil)
	checkCliDefault("cli.YakCode", []*ssaapi.Type{String}, nil)
	checkCliDefault("cli.Text", []*ssaapi.Type{String}, nil)
	checkCliDefault("cli.Json", []*ssaapi.Type{String}, nil)
	return ret
}

// 检查参数名是否重复和参数名是否符合规范
func RuleCliParamName(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("cli param name check")
	// tag := "SSA-cli-paramName"
	cliFuncNames := []string{
		"cli.String",
		"cli.StringSlice",
		"cli.Bool",
		"cli.Int",
		"cli.Integer",
		"cli.Double",
		"cli.Float",
		"cli.Url",
		"cli.Urls",
		"cli.Port",
		"cli.Ports",
		"cli.Net",
		"cli.Network",
		"cli.Host",
		"cli.Hosts",
		"cli.File",
		"cli.FileOrContent",
		"cli.LineDict",
		"cli.YakitPlugin",
		"cli.HTTPPacket",
		"cli.Have",
		"cli.YakCode",
		"cli.Text",
		"cli.Json",
	}

	paramLineMap := make(map[string]int)
	for _, funcName := range cliFuncNames {
		prog.Ref(funcName).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			firstField := v.GetOperand(1)
			if firstField == nil {
				return
			}
			paramName := firstField.String()
			rawParamName := paramName
			if unquoted, err := strconv.Unquote(paramName); err == nil {
				rawParamName = unquoted
			}
			if _, ok := paramLineMap[paramName]; !ok {
				paramLineMap[paramName] = int(v.GetRange().GetStart().GetLine())
				if !utils.MatchAllOfRegexp(rawParamName, `^[a-zA-Z0-9_-]+$`) {
					ret.NewError(ErrorStrInvalidParamName(rawParamName), firstField)
				}
			} else {
				ret.NewError(ErrorStrSameParamName(rawParamName, paramLineMap[paramName]), firstField)
			}
		})
	}
	return ret
}

// 检查是否在最后面调用了 cli.check
func RuleCliCheck(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("cli.check in the end")
	cliFuncNames := []string{
		"cli.String",
		"cli.StringSlice",
		"cli.Bool",
		"cli.Int",
		"cli.Integer",
		"cli.Double",
		"cli.Float",
		"cli.Url",
		"cli.Urls",
		"cli.Port",
		"cli.Ports",
		"cli.Net",
		"cli.Network",
		"cli.Host",
		"cli.Hosts",
		"cli.File",
		"cli.FileOrContent",
		"cli.LineDict",
		"cli.YakitPlugin",
		"cli.check",
		"cli.HTTPPacket",
		"cli.Have",
		"cli.YakCode",
		"cli.Text",
		"cli.UI",
		"cli.Json",
	}

	var (
		lastCallValue    *ssaapi.Value
		lastCallPosition int64
		lastCallName     string
	)

	for _, funcName := range cliFuncNames {
		prog.Ref(funcName).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			startLine := int64(v.GetRange().GetStart().GetLine())
			if startLine > lastCallPosition {
				lastCallPosition = startLine
				lastCallName = funcName
				lastCallValue = v
			}
		})
	}

	if lastCallName != "cli.check" && lastCallValue != nil {
		ret.NewError(ErrorStrNotCallCliCheck(), lastCallValue)
	}
	return ret
}

func ErrorStrNotCallCliCheck() string {
	return "please call cli.check as the last statement after all other cli standard library calls"
}

func ErrorStrSameParamName(name string, line int) string {
	return fmt.Sprintf("parameter [%s] already defined at line %d", name, line)
}

func ErrorStrInvalidParamName(name string) string {
	return fmt.Sprintf("parameter [%s] should be letters or numbers or _ or -", name)
}
