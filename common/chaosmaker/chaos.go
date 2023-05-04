package chaosmaker

import (
	"github.com/yaklang/yaklang/common/suricata"
	"sync"
)

/*
	ChaosMaker is working for reversing suricata rules to traffic!

	Chaos means interface for proto
*/

type chaosHandler struct {
	// Generate
	Generator func(maker *ChaosMaker, chaosRule *ChaosMakerRule, rule *suricata.Rule) chan *ChaosTraffic
	//
	MatchBytes func(i interface{}) bool
}

// chaosMap means registered map for rule
//
//	act: map[string]chaosHandler
var chaosMap = new(sync.Map)
