package rule

import (
	"bufio"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/yaklang/antlr/v4"
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
	return parseRules(data, false, envs...)
}

func ParseStrictRule(data string, envs ...string) ([]*Rule, error) {
	return parseRules(data, true, envs...)
}

func parseRules(data string, strict bool, envs ...string) ([]*Rule, error) {
	config := config2.DefaultConfig.Clone()
	var envErrors []error
	for _, env := range envs {
		name, value, ok := strings.Cut(env, "=")
		if !ok {
			envErrors = append(envErrors, fmt.Errorf("suricata env %q must use key=value format", env))
			continue
		}
		name = strings.Trim(strings.TrimSpace(name), "${}")
		if name == "" {
			envErrors = append(envErrors, fmt.Errorf("suricata env %q has an empty name", env))
			continue
		}
		if err := config.AddVar(name, strings.TrimSpace(value)); err != nil {
			envErrors = append(envErrors, fmt.Errorf("add suricata env %s: %w", name, err))
		}
	}

	compileRaw, malformedRules := preprocessRules(data)
	input := antlr.NewInputStream(compileRaw)
	errListener := antlr4util.NewErrorListener()
	lexer := rule.NewSuricataRuleLexer(input)
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := rule.NewSuricataRuleParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)

	ctx, ok := parser.Rules().(*rule.RulesContext)
	if !ok || ctx == nil {
		return nil, errors.Join(append(envErrors, errors.New("suricata parser returned no rule context"))...)
	}
	if strict && len(errListener.GetErrors()) > 0 {
		return nil, errors.Join(append(envErrors, errListener.Error())...)
	}

	visitor := &RuleSyntaxVisitor{Raw: []byte(data), CompileRaw: compileRaw, Config: config}
	visitor.VisitRules(ctx)
	for _, parsed := range visitor.Rules {
		ParseRuleMetadata(parsed)
	}

	parseErrors := append([]error(nil), envErrors...)
	if strict || len(visitor.Rules) == 0 {
		parseErrors = append(parseErrors, malformedRules...)
		parseErrors = append(parseErrors, visitor.Errors...)
		if syntaxErr := errListener.Error(); syntaxErr != nil {
			parseErrors = append(parseErrors, syntaxErr)
		}
	}
	if len(visitor.Rules) == 0 && len(parseErrors) == 0 {
		if strings.TrimSpace(compileRaw) == "" {
			// Comment-only rule files are valid empty rule sets. Suricata
			// distributions ship several of them (license text or deleted
			// rule history), and directory loading must not fail on them.
			return nil, nil
		}
		parseErrors = append(parseErrors, errors.New("no valid suricata rules found"))
	}
	if len(parseErrors) > 0 {
		return visitor.Rules, errors.Join(parseErrors...)
	}
	return visitor.Rules, nil
}

func preprocessRules(data string) (string, []error) {
	var buf strings.Builder
	var malformed []error
	reader := bufio.NewReader(strings.NewReader(data))
	lineNumber := 0
	for {
		line, err := utils.BufioReadLineString(reader)
		if err != nil {
			break
		}
		lineNumber++
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || line == "" {
			buf.WriteByte('\n')
			continue
		}
		// Suricata's PCRE option parser does not treat quotes inside the regex
		// as string delimiters. Escape them for the grammar so they stay inside
		// MODE_STRING; unquotePCREString restores the original expression.
		line = escapeInnerPCREQuotes(line)
		// A rule with an unterminated quoted value leaves the ANTLR lexer in
		// MODE_STRING and swallows every following line until the next quote.
		// Drop only the malformed physical line so valid rules in the same file
		// are still parsed.
		if isRuleActionLine(line) && !hasBalancedRuleQuotes(line) {
			malformed = append(malformed, fmt.Errorf("line %d: rule has unbalanced quotes", lineNumber))
			buf.WriteByte('\n')
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return buf.String(), malformed
}

// escapeInnerPCREQuotes protects quote characters embedded in a pcre value
// from the grammar's quoted-string lexer. The closing quote is the last quote
// before the option-terminating semicolon; all earlier quotes belong to the
// regular expression.
func escapeInnerPCREQuotes(line string) string {
	search := 0
	for search < len(line) {
		relative := strings.Index(strings.ToLower(line[search:]), "pcre:")
		if relative < 0 {
			break
		}
		start := search + relative
		if start > 0 {
			previous := start - 1
			for previous >= 0 && (line[previous] == ' ' || line[previous] == '\t') {
				previous--
			}
			if previous >= 0 && line[previous] != ';' && line[previous] != '(' {
				search = start + len("pcre:")
				continue
			}
		}
		quote := start + len("pcre:")
		if quote >= len(line) || line[quote] != '"' {
			search = quote
			continue
		}
		end := optionEnd(line, quote+1)
		relativeClose := strings.LastIndex(line[quote+1:end], `"`)
		if relativeClose < 0 {
			break
		}
		close := quote + 1 + relativeClose
		body := line[quote+1 : close]
		escaped := escapeUnescapedQuotes(body)
		if escaped == body {
			search = close + 1
			continue
		}
		line = line[:quote+1] + escaped + line[close:]
		search = quote + 1 + len(escaped) + 1
	}
	return line
}

func optionEnd(line string, start int) int {
	escaped := false
	for index := start; index < len(line); index++ {
		switch {
		case escaped:
			escaped = false
		case line[index] == '\\':
			escaped = true
		case line[index] == ';':
			return index
		}
	}
	return len(line)
}

func escapeUnescapedQuotes(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	backslashes := 0
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\\':
			backslashes++
			builder.WriteByte(value[index])
		case '"':
			if backslashes%2 == 0 {
				builder.WriteByte('\\')
			}
			builder.WriteByte(value[index])
			backslashes = 0
		default:
			backslashes = 0
			builder.WriteByte(value[index])
		}
	}
	return builder.String()
}

func isRuleActionLine(line string) bool {
	end := strings.IndexAny(line, " \t")
	if end < 0 {
		return false
	}
	switch line[:end] {
	case "alert", "pass", "drop", "reject", "rejectsrc", "rejectdst", "rejectboth":
		return true
	default:
		return false
	}
}

func hasBalancedRuleQuotes(line string) bool {
	quotes := 0
	escaped := false
	for index := 0; index < len(line); index++ {
		switch line[index] {
		case '\\':
			escaped = !escaped
		case '"':
			if !escaped {
				quotes++
			}
			escaped = false
		default:
			escaped = false
		}
	}
	return quotes%2 == 0
}

func ParseRuleMetadata(rule *Rule) {
	defer func() {
		if recover() != nil {
			return
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
