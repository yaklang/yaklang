package match

import (
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
)

func fastPatternCopy(c *rule.ContentRule) *rule.ContentRule {
	var transforms []string
	if len(c.Transforms) > 0 {
		transforms = append([]string(nil), c.Transforms...)
	}
	return &rule.ContentRule{
		Content:    utils.CopyBytes(c.Content),
		Nocase:     c.Nocase,
		Modifier:   c.Modifier,
		DotPrefix:  c.DotPrefix,
		Transforms: transforms,
		RawBytes:   c.RawBytes,
	}
}
