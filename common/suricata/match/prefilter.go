package match

import (
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
)

func fastPatternCopy(c *rule.ContentRule) *rule.ContentRule {
	return &rule.ContentRule{
		Content:  utils.CopyBytes(c.Content),
		Nocase:   c.Nocase,
		Modifier: c.Modifier,
	}
}
