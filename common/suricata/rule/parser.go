package rule

import (
	"bufio"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	rule "github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
	"strings"
)

var presetEnv = map[string]string{
	"HOME_NET": utils.GetLocalIPAddress(),
}

func init() {
	for _, varName := range []string{"HTTP_SERVERS", "SMTP_SERVERS", "SQL_SERVERS", "DNS_SERVERS", "TELNET_SERVERS"} {
		presetEnv[varName] = presetEnv["HOME_NET"]
	}
}

func Parse(data string, envs ...string) ([]*Rule, error) {
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
	//for _, t := range lexer.GetAllTokens() {
	//	fmt.Println(t)
	//}
	v := &RuleSyntaxVisitor{Raw: []byte(data), CompileRaw: compileRaw}
	v.Environment = make(map[string]string)
	for k, val := range presetEnv {
		v.Environment[k] = val
	}
	for _, e := range envs {
		before, after, cut := strings.Cut(e, "=")
		if !cut {
			log.Warnf("env input:[%v] cannot parse as key=value", e)
			continue
		}
		v.Environment[before] = after
	}
	v.VisitRules(parser.Rules().(*rule.RulesContext))
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
