package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/pcapx"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"sync"
)

/*
	ChaosMaker is working for reversing suricata rules to traffic!

	Chaos means interface for proto
*/

type chaosHandler interface {
	Generator(maker *ChaosMaker, chaosRule *rule.Storage, rule *surirule.Rule) chan *pcapx.ChaosTraffic
	MatchBytes(i any) bool
}

// chaosMap means registered map for rule
//
//	act: map[string]chaosHandler
var chaosMap sync.Map
