package sfcompletion

import (
	_ "embed"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"strconv"
	"strings"
)

//go:embed desc_completion_prompt.md
var CompleteRuleDescPrompt string

func generateCompleteRuleDescPrompt(fileName, ruleContent string, fileds ...sfvm.SFDescKeyType) string {
	allFields := lo.Map(fileds, func(item sfvm.SFDescKeyType, index int) string {
		return string(item)
	})
	filedStr := strings.Join(allFields, ", ")
	return fmt.Sprintf(CompleteRuleDescPrompt, filedStr, fileName, ruleContent)
}

func completeRuleDesc(fileName, ruleContent string, descFields []sfvm.SFDescKeyType, aiConfig ...aispec.AIConfigOption) (map[string]any, error) {
	prompt := generateCompleteRuleDescPrompt(fileName, ruleContent, descFields...)
	client := ai.GetAI("siliconflow", aiConfig...)
	stream, err := client.ChatStream(prompt)
	if err != nil {
		return nil, err
	}

	content, err := io.ReadAll(stream)
	if err != nil {
		return nil, err
	}
	data := string(content)
	fields := lo.SliceToMap(descFields, func(item sfvm.SFDescKeyType) (string, any) {
		return string(item), nil
	})

	extracted, err := aispec.ExtractFromResult(data, fields)
	if err != nil {
		return nil, err
	}
	return extracted, nil
}

// CompleteInfoDesc 用于给sf rule文件的desc中信息项内容补全，包括title、title_zh、desc、solution等
func CompleteInfoDesc(fileName, ruleContent string, aiConfig ...aispec.AIConfigOption) (string, error) {
	fields := sfvm.GetSupplyInfoDescKeyType()

	maxRetries := 3
	var (
		desc map[string]any
		err  error
	)
	for i := 0; i < maxRetries; i++ {
		desc, err = completeRuleDesc(fileName, ruleContent, fields, aiConfig...)
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", fmt.Errorf("complete rule desc error: %w", err)
	}

	handler := func(key, value string) string {
		typ := sfvm.ValidDescItemKeyType(key)
		if ret, ok := desc[string(typ)]; ok {
			newValue := codec.AnyToString(ret)
			if newValue == "" {
				return value
			}
			if sfvm.IsComplexInfoDescType(typ) {
				upperKey := strings.ToUpper(key)
				newValue = fmt.Sprintf("<<<%s\n%s\n%s", upperKey, newValue, upperKey)
				return newValue
			}
			_, err := strconv.ParseInt(newValue, 10, 64)
			if err == nil {
				return newValue
			}
			_, err = strconv.ParseInt(newValue, 8, 64)
			if err == nil {
				return newValue
			}
			_, err = strconv.ParseInt(newValue, 16, 64)
			if err == nil {
				return newValue
			}
			return strconv.Quote(newValue)
		}
		return value
	}

	var opts []sfvm.RuleFormatOption
	opts = append(opts,
		sfvm.RuleFormatWithRequireInfoDescKeyType(fields...),
		sfvm.RuleFormatWithInfoDescHandler(handler),
	)
	content, err := sfvm.FormatRule(ruleContent, opts...)
	if err != nil {
		return "", err
	}
	return content, nil
}
