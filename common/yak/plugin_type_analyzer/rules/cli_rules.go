package rules

import (
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// 检查 cli.setDefault 设置的默认值是否符合规范
func RuleCliDefault(prog *ssaapi.Program) {
	tag := "SSA-cli-setDefault"
	checkCliDefault := func(funcName string, typ *ssaapi.Type, checkCallBack func(funcName string, v *ssaapi.Value) (string, bool)) {
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
				fieldTyp := field.GetType()
				if !fieldTyp.Compare(typ) {
					field.NewError(tag, fmt.Sprintf("%s want [%s] type, but got [%s] type", funcName, typ, fieldTyp))
					break
				}

				if checkCallBack != nil {
					message, ok := checkCallBack(funcName, field)
					if !ok {
						field.NewError(tag, message)
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
		for _, port := range ports {
			if port <= 0 || port > 65535 {
				return fmt.Sprintf("%s want valid port, but got %d", funcName, port), false
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

	checkCliDefault("cli.String", ssaapi.String, nil)
	checkCliDefault("cli.StringSlice", ssaapi.String, nil)
	checkCliDefault("cli.Bool", ssaapi.Boolean, nil)
	checkCliDefault("cli.Int", ssaapi.Number, nil)
	checkCliDefault("cli.Integer", ssaapi.Number, nil)
	checkCliDefault("cli.Double", ssaapi.Number, nil) // 需要区分 int 和 double
	checkCliDefault("cli.Float", ssaapi.Number, nil)  // 需要区分 int 和 double
	checkCliDefault("cli.Url", ssaapi.Any, urlsCallback)
	checkCliDefault("cli.Urls", ssaapi.Any, urlsCallback)
	checkCliDefault("cli.Port", ssaapi.Any, portsCallback)
	checkCliDefault("cli.Ports", ssaapi.Any, portsCallback)
	checkCliDefault("cli.Net", ssaapi.Any, hostCallback)
	checkCliDefault("cli.Network", ssaapi.Any, hostCallback)
	checkCliDefault("cli.Host", ssaapi.Any, hostCallback)
	checkCliDefault("cli.Hosts", ssaapi.Any, hostCallback)
	checkCliDefault("cli.File", ssaapi.String, fileCallback)
	checkCliDefault("cli.FileOrContent", ssaapi.String, fileOrContentCallback)
	checkCliDefault("cli.LineDict", ssaapi.String, fileOrContentCallback)
	checkCliDefault("cli.YakitPlugin", ssaapi.String, fileCallback)
}

// 检查参数名是否重复和参数名是否符合规范
func RuleCliParamName(prog *ssaapi.Program) {
	tag := "SSA-cli-paramName"
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
				paramLineMap[paramName] = v.GetPosition().StartLine
				if !utils.MatchAllOfRegexp(rawParamName, `^[a-zA-Z0-9]+$`) {
					firstField.NewError(tag, fmt.Sprintf("parameter [%s] should be letters or numbers", rawParamName))
				}
			} else {
				firstField.NewError(tag, fmt.Sprintf("parameter [%s] already defined at line %d", rawParamName, paramLineMap[paramName]))
			}
		})
	}
}

// 检查是否在最后面调用了 cli.check
func RuleCliCheck(prog *ssaapi.Program) {
	tag := "SSA-cli-check"
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
	}

	var (
		lastCallValue    *ssaapi.Value
		lastCallPosition int
		lastCallName     string
	)

	for _, funcName := range cliFuncNames {
		prog.Ref(funcName).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			startLine := v.GetPosition().StartLine
			if startLine > lastCallPosition {
				lastCallPosition = startLine
				lastCallName = funcName
				lastCallValue = v
			}
		})
	}

	if lastCallName != "cli.check" && lastCallValue != nil {
		lastCallValue.NewError(tag, NotCallCliCheck())
	}
}

func NotCallCliCheck() string {
	return "please call cli.check as the last statement after all other cli standard library calls"
}
