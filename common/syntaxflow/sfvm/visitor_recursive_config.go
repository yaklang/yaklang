package sfvm

import "github.com/yaklang/yaklang/common/syntaxflow/sf"

func (v *SyntaxFlowVisitor) VisitRecursiveConfig(i *sf.RecursiveConfigContext) []*ConfigItem {
	if i == nil {
		return nil
	}
	var res []*ConfigItem
	for _, i := range i.AllRecursiveConfigItem() {
		item, ok := i.(*sf.RecursiveConfigItemContext)
		if !ok {
			continue
		}
		key := item.Identifier()
		configItem := &ConfigItem{Key: key.GetText()}
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
