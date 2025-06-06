package sfvm

import (
	"fmt"
	"regexp"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
)

func (v *SyntaxFlowVisitor) VisitRecursiveConfig(i *sf.ConfigContext) []*RecursiveConfigItem {
	if i == nil {
		return nil
	}
	var res []*RecursiveConfigItem
	for _, i := range i.AllRecursiveConfigItem() {
		item, ok := i.(*sf.RecursiveConfigItemContext)
		if !ok {
			continue
		}
		key := item.Identifier()
		configKey := FormatRecursiveConfigKey(key.GetText())
		if configKey == RecursiveConfig_NULL {
			log.Warnf("invalid recursive config key: %s", key.GetText())
		}
		configItem := &RecursiveConfigItem{Key: string(configKey)}
		value := item.RecursiveConfigItemValue().(*sf.RecursiveConfigItemValueContext)
		if rule := value.FilterStatement(); rule != nil {
			configItem.SyntaxFlowRule = true

			{
				start := rule.GetStart()
				end := rule.GetStop()
				input := rule.GetStart().GetInputStream()
				text := input.GetTextFromInterval(antlr.NewInterval(
					start.GetStart(), end.GetStop(),
				))
				configItem.Value = text
			}

		} else if doc := value.HereDoc(); doc != nil {
			configItem.SyntaxFlowRule = true
			configItem.Value = v.VisitHereDoc(doc)
		} else {
			configItem.Value = yakunquote.TryUnquote(value.GetText())
		}
		res = append(res, configItem)
	}
	return res
}

var configKeyRegexp = regexp.MustCompile(`[a-zA-Z_][-a-zA-Z0-9_]*`)

func (v *SyntaxFlowVisitor) VisitNativeCallActualParams(i *sf.NativeCallActualParamsContext) []*RecursiveConfigItem {
	if i == nil {
		return nil
	}
	var res []*RecursiveConfigItem
	var count = 0
	for _, i := range i.AllNativeCallActualParam() {
		item, ok := i.(*sf.NativeCallActualParamContext)
		if !ok {
			continue
		}
		var configKey string
		if item.NativeCallActualParamKey() != nil {
			key := item.NativeCallActualParamKey()
			origin := key.GetText()
			configKey = yakunquote.TryUnquote(origin)
			if !configKeyRegexp.MatchString(configKey) {
				log.Infof("invalid native call key: %s", configKey)
				configKey = fmt.Sprint(count)
				count++
			}
		} else {
			configKey = fmt.Sprint(count)
			count++
		}

		configItem := &RecursiveConfigItem{Key: configKey}
		value := item.NativeCallActualParamValue()
		if docContainer, ok := value.(*sf.NativeCallActualParamValueContext); ok {
			if docContainer.HereDoc() != nil {
				configItem.Value = v.VisitHereDoc(docContainer.HereDoc())
			} else {
				configItem.Value = yakunquote.TryUnquote(value.GetText())
			}
		} else {
			configItem.Value = yakunquote.TryUnquote(value.GetText())
		}
		res = append(res, configItem)
	}
	return res
}

func (v *SyntaxFlowVisitor) VisitHereDoc(i any) string {
	if i == nil {
		return ""
	}
	item, ok := i.(*sf.HereDocContext)
	if !ok {
		return ""
	}
	if item.LfHereDoc() != nil {
		doc, ok := item.LfHereDoc().(*sf.LfHereDocContext)
		if ok && doc.LfText() != nil {
			return doc.LfText().GetText()
		}
		return ""
	}

	if item.CrlfHereDoc() != nil {
		doc, ok := item.CrlfHereDoc().(*sf.CrlfHereDocContext)
		if ok {
			return doc.CrlfText().GetText()
		}
		return ""
	}
	return ""
}
