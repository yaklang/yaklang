package core

import "github.com/yaklang/yaklang/common/log"

type TaggedCallLowering struct {
	Symbol string
	Arity  int
}

var taggedCallLowerings = make(map[string]TaggedCallLowering)

func RegisterTaggedCallLowering(tag string, lowering TaggedCallLowering) {
	if tag == "" {
		log.Warnf("skip tagged call lowering registration with empty tag")
		return
	}
	if lowering.Symbol == "" {
		log.Warnf("skip tagged call lowering registration %q with empty symbol", tag)
		return
	}
	if _, exists := taggedCallLowerings[tag]; exists {
		log.Warnf("skip duplicate tagged call lowering registration %q", tag)
		return
	}
	taggedCallLowerings[tag] = lowering
}

func LookupTaggedCallLowering(tag string) (TaggedCallLowering, bool) {
	lowering, ok := taggedCallLowerings[tag]
	return lowering, ok
}
