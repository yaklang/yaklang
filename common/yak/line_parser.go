package yak

import (
	"bufio"
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"io"
)

type TextHandlingScript struct {
	RuleID         string `json:"rule_id" yaml:"rule_id"`
	MatchingScript string `json:"matching_script" yaml:"matching_script"`
	ParsingScript  string `json:"parsing_script" yaml:"parsing_script"`
}

type TextParser struct {
	Scripts map[string]*TextHandlingScript

	engine *antlr4yak.Engine
}

// NewTextParser 创建一个新的文本解析器
func NewTextParser() *TextParser {
	parser := &TextParser{
		Scripts: map[string]*TextHandlingScript{
			"fallback": {
				RuleID:         "fallback",
				MatchingScript: "MATCHED=true",
				ParsingScript:  `JSON_DATA=str.JsonToMapList(FILE_LINE)`,
			},
		},
		engine: yaklang.New(),
	}

	return parser
}

func (t *TextParser) ParseLine(r io.Reader, handler func(line string, r map[string]string, data []map[string]string)) error {
	lineScanner := bufio.NewScanner(r)
	lineScanner.Split(bufio.ScanLines)

	var (
		matchedRule string
	)

	for lineScanner.Scan() {
		line := lineScanner.Text()
		t.engine.SetVar("FILE_LINE", line)

		// 自动选择合适的规则来解析
		if matchedRule == "" {
			for name, subRule := range t.Scripts {
				_ = name
				err := t.engine.SafeEval(context.Background(), subRule.MatchingScript)
				if err != nil {
					continue
				}

				if matchedRaw, ok := t.engine.GetVar("MATCHED"); ok {
					if matched, typeOk := matchedRaw.(bool); typeOk && matched {
						// 规则匹配上了
						matchedRule = subRule.ParsingScript
					}
				}
			}
		}

		if matchedRule == "" {
			return utils.Errorf("parse text failed: %v", "no proper rule")
		}

		err := t.engine.SafeEval(context.Background(), matchedRule)
		if err != nil {
			return utils.Errorf("rule[%s] failed for handling: %s", matchedRule, err)
		}

		var result map[string]string = map[string]string{}
		if raw, ok := t.engine.GetVar("RESULT"); ok {
			if strRawMap, tOk := raw.(map[string]string); tOk {
				if strRawMap == nil || len(strRawMap) <= 0 {
					if utils.InDebugMode() {
						log.Infof("[%s] parse [%v] no results", matchedRule, line)
					}
					continue
				}
				result = utils.MergeStringMap(result, strRawMap)
			}
		}

		var jsonData []map[string]string
		if raw, ok := t.engine.GetVar("JSON_DATA"); ok {
			if strRawMaps, tOk := raw.([]map[string]string); tOk {
				jsonData = append(jsonData, strRawMaps...)
			}
		}

		if len(result) > 0 || len(jsonData) > 0 {
			handler(line, result, jsonData)
		} else {
			if utils.InDebugMode() {
				log.Warnf("no results for [%s] parsing %s", matchedRule, line)
			}
		}
	}
	return nil
}
