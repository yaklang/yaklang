package parser

import "sync"

var templateDepthMap = new(sync.Map)

func (p *YaklangLexer) DecreaseTemplateDepth() {
	iRaw, ok := templateDepthMap.Load(p)
	if ok {
		i := iRaw.(uint64)
		if i > 0 {
			if i-1 == 0 {
				templateDepthMap.Delete(p)
			} else {
				templateDepthMap.Store(p, i-1)
			}
		}
	} else {
		panic("BUG: yaklang lexer decrease template depth failed... unbalanced depth")
	}
}
func (p *YaklangLexer) IncreaseTemplateDepth() {
	iRaw, ok := templateDepthMap.Load(p)
	if ok {
		templateDepthMap.Store(p, iRaw.(uint64)+1)
	} else {
		templateDepthMap.Store(p, uint64(1))
	}
}
func (p *YaklangLexer) IsInTemplateString() bool {
	_, ok := templateDepthMap.Load(p)
	return ok
}
