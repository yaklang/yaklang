package rule

import (
	"bufio"
	"errors"
	"reflect"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	config2 "github.com/yaklang/yaklang/common/suricata/config"
	rule "github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

// ParseSuricata 解析 Suricata 规则字符串，返回解析后的规则对象列表。
// 它接受一个包含一条或多条 Suricata 规则的字符串，以及可选的环境变量配置。
// 支持标准的 Suricata 规则语法，包括 action、protocol、地址、端口和规则选项。
// 环境变量格式为 "key=value"，用于替换规则中的变量（如 $HOME_NET、$EXTERNAL_NET）。
// Example:
// ```
// 解析单条规则
// rules, err = suricata.ParseSuricata(`alert tcp any any -> any 80 (msg:"HTTP Attack"; content:"attack"; sid:1001;)`)
// 解析多条规则
// rules, err = suricata.ParseSuricata(`
// alert tcp $HOME_NET any -> $EXTERNAL_NET 80 (msg:"Outbound HTTP"; sid:1001;)
// alert dns any any -> any 53 (msg:"DNS Query"; sid:1002;)
// `)
// 使用自定义环境变量
// rules, err = suricata.ParseSuricata(ruleContent, "HOME_NET=192.168.1.0/24", "EXTERNAL_NET=any")
// ```
func Parse(data string, envs ...string) ([]*Rule, error) {
	config, err := config2.ParseSuricataConfig(config2.DefaultConfigYaml)
	if err != nil {
		log.Errorf("initing suricata default config failed: %v", err)
	}
	var buf strings.Builder
	var dataBuf = bufio.NewReader(strings.NewReader(data))
	for {
		line, err := utils.BufioReadLineString(dataBuf)
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || line == "" {
			buf.WriteByte('\n')
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	compileRaw := buf.String()
	lexer := rule.NewSuricataRuleLexer(antlr.NewInputStream(compileRaw))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := rule.NewSuricataRuleParser(tokenStream)
	parser.RemoveErrorListeners()
	errListener := antlr4util.NewErrorListener()
	parser.AddErrorListener(errListener)
	v := &RuleSyntaxVisitor{Raw: []byte(data), CompileRaw: compileRaw, Config: config}
	for _, e := range envs {
		before, after, cut := strings.Cut(e, "=")
		if !cut {
			log.Warnf("env input:[%v] cannot parse as key=value", e)
			continue
		}
		v.Config.AddVar(before, after)
	}
	ruleCtx := parser.Rules().(*rule.RulesContext)
	v.VisitRules(ruleCtx)
	for _, r := range v.Rules {
		ParseRuleMetadata(r)
	}
	if len(v.Rules) > 0 {
		return v.Rules, nil
	} else {
		return nil, v.MergeErrors()
	}
}
func ParseStrictRule(data string, envs ...string) ([]*Rule, error) {
	config, err := config2.ParseSuricataConfig(config2.DefaultConfigYaml)
	if err != nil {
		log.Errorf("initing suricata default config failed: %v", err)
	}
	var buf strings.Builder
	var dataBuf = bufio.NewReader(strings.NewReader(data))
	for {
		line, err := utils.BufioReadLineString(dataBuf)
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || line == "" {
			buf.WriteByte('\n')
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	compileRaw := buf.String()
	lexer := rule.NewSuricataRuleLexer(antlr.NewInputStream(compileRaw))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := rule.NewSuricataRuleParser(tokenStream)
	parser.RemoveErrorListeners()
	errListener := antlr4util.NewErrorListener()
	parser.AddErrorListener(errListener)
	v := &RuleSyntaxVisitor{Raw: []byte(data), CompileRaw: compileRaw, Config: config}
	for _, e := range envs {
		before, after, cut := strings.Cut(e, "=")
		if !cut {
			log.Warnf("env input:[%v] cannot parse as key=value", e)
			continue
		}
		v.Config.AddVar(before, after)
	}
	ruleCtx := parser.Rules().(*rule.RulesContext)
	if len(errListener.GetErrors()) > 0 {
		return nil, errors.New(errListener.GetErrorString())
	}
	v.VisitRules(ruleCtx)
	for _, r := range v.Rules {
		ParseRuleMetadata(r)
	}
	if len(v.Rules) > 0 {
		return v.Rules, nil
	} else {
		return nil, v.MergeErrors()
	}
}

func ParseRuleMetadata(rule *Rule) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("parse rule metadata failed: %s", err)
		}
	}()
	if rule == nil {
		return
	}
	for _, meta := range rule.Metadata {
		for _, item := range strings.Split(meta, ",") {
			info := strings.Split(strings.TrimSpace(item), " ")
			if len(info) != 2 {
				continue
			}
			t := reflect.TypeOf(rule).Elem()
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				tag := field.Tag.Get("json")
				if tag == info[0] {
					if info[0] == "CVE" {
						info[1] = strings.Replace(info[1], "_", "-", -1)
					}
					if info[0] == "updated_at" {
						info[1] = strings.Replace(info[1], "_", "-", -1)
					}
					if info[0] == "created_at" {
						info[1] = strings.Replace(info[1], "_", "-", -1)
					}
					if info[0] == "reviewed_at" {
						info[1] = strings.Replace(info[1], "_", "-", -1)
					}
					reflect.ValueOf(rule).Elem().Field(i).SetString(info[1])
				}
			}
		}
	}
}
