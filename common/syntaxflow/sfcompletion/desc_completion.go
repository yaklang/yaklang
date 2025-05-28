package sfcompletion

import (
	_ "embed"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"regexp"
	"strings"
)

//go:embed prompt/basis_desc_completion_prompt.md
var BasicDescCompletionPrompt string

//go:embed prompt/complex_desc_completion_prompt.md
var ComplexDescCompletionPrompt string

var markdownExtractor = regexp.MustCompile("(?s)```markdown(.*)```")

// generateBasisDescCompletionPrompt 创建规则的基础描述补全promt
func generateBasisDescCompletionPrompt(fileName, ruleContent string, fileds ...sfvm.SFDescKeyType) string {
	allFields := lo.Map(fileds, func(item sfvm.SFDescKeyType, index int) string {
		return string(item)
	})
	filedStr := strings.Join(allFields, ", ")
	return fmt.Sprintf(BasicDescCompletionPrompt, filedStr, fileName, ruleContent)
}

func generateComplexDescCompletionPrompt(fileName, ruleContent string, filed sfvm.SFDescKeyType) string {
	return fmt.Sprintf(ComplexDescCompletionPrompt, string(filed), fileName, ruleContent)
}

func (rc *RuleCompletion) getComplexDescCompletionInfo(
	descField sfvm.SFDescKeyType,
) (string, error) {
	prompt := generateComplexDescCompletionPrompt(rc.FileUrl, rc.RuleContent, descField)
	stream, err := ai.ChatStream(prompt, rc.AIConfig...)
	if err != nil {
		return "", err
	}

	raw, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}
	data := string(raw)
	// 提取markdown内容
	match := markdownExtractor.FindStringSubmatch(data)
	result := ""
	if len(match) == 2 {
		result = match[1]
	}
	if result == "" {
		return "", utils.Error("getComplexDescCompletionInfo failed:no markdown content found in the response")
	}
	return result, nil
}

// getBasisDescCompletionInfo 得到补全后规则描述基础信息
func (rc *RuleCompletion) getBasisDescCompletionInfo(
	descFields []sfvm.SFDescKeyType,
) (map[string]any, error) {
	prompt := generateBasisDescCompletionPrompt(rc.FileUrl, rc.RuleContent, descFields...)
	stream, err := ai.ChatStream(prompt, rc.AIConfig...)
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

// CompleteRuleDesc 用于给sf rule文件的desc中信息项内容补全，包括title、title_zh、desc、solution等
func CompleteRuleDesc(
	fileName, ruleContent string,
	aiConfig ...aispec.AIConfigOption,
) (string, error) {
	rc := NewRuleCompletion(fileName, ruleContent, aiConfig...)
	basicType := sfvm.GetBasisSupplyInfoDescKeyType()
	basicDescInfo, err := rc.getBasisDescCompletionInfo(basicType)
	if err != nil {
		return "", err
	}

	handler := func(key, value string) string {
		typ := sfvm.ValidDescItemKeyType(key)
		ret, ok := basicDescInfo[string(typ)]
		if ok {
			// 简单信息补全
			return codec.AnyToString(ret)
		}
		if sfvm.IsComplexInfoDescType(typ) {
			//复杂信息补全,少于300字的内容需要补全
			if len(value) < 300 {
				newValue, err := rc.getComplexDescCompletionInfo(typ)
				if err != nil {
					log.Errorf("complete rule desc error:%s", err.Error())
					return value
				}
				return newValue
			}
		}
		// 如果没有补全信息，直接返回原值
		return value
	}

	var opts []sfvm.RuleFormatOption
	opts = append(opts,
		sfvm.RuleFormatWithRequireDescKeyType(sfvm.GetSupplyInfoDescKeyType()...),
		sfvm.RuleFormatWithDescHandler(handler),
	)
	content, err := sfvm.FormatRule(ruleContent, opts...)
	if err != nil {
		return "", err
	}
	return content, nil
}
