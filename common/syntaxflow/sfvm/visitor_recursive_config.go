package sfvm

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
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
		configItem := &RecursiveConfigItem{Key: configKey}
		value := item.RecursiveConfigItemValue().(*sf.RecursiveConfigItemValueContext)
		if rule := value.FilterStatement(); rule != nil {
			configItem.SyntaxFlowRule = true
			configItem.Value = rule.GetText()
		} else {
			configItem.Value = value.GetText()
		}
		res = append(res, configItem)
	}
	return res
}
