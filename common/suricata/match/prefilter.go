package match

import (
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
)

func fastPatternCopy(c *rule.ContentRule) *rule.ContentRule {
	var extra []string
	copy(extra, c.ExtraFlags)
	return &rule.ContentRule{
		Negative:     c.Negative,
		Content:      utils.CopyBytes(c.Content),
		Nocase:       c.Nocase,
		StartsWith:   c.StartsWith,
		EndsWith:     c.EndsWith,
		RawBytes:     c.RawBytes,
		IsDataAt:     c.IsDataAt,
		BSize:        c.BSize,
		DSize:        c.DSize,
		ByteTest:     c.ByteTest,
		ByteMath:     c.ByteMath,
		ByteJump:     c.ByteJump,
		ByteExtract:  c.ByteExtract,
		RPC:          c.RPC,
		PCRE:         c.PCRE,
		FastPattern:  c.FastPattern,
		FlowBits:     c.FlowBits,
		FlowInt:      c.FlowInt,
		XBits:        c.XBits,
		NoAlert:      c.NoAlert,
		Base64Decode: c.Base64Decode,
		Base64Data:   c.Base64Data,
		ExtraFlags:   extra,
		Modifier:     c.Modifier,
	}
}
