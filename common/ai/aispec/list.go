package aispec

import "github.com/yaklang/yaklang/common/utils/omap"

var list = omap.NewOrderedMap(map[string]func() AIGateway{})

func Register(name string, gateway func() AIGateway) {
	if gateway == nil {
		return
	}
	list.Set(name, gateway)
}

func Lookup(name string) (AIGateway, bool) {
	creator, ok := list.Get(name)
	if !ok {
		return nil, false
	}
	return creator(), true
}

func RegisteredAIGateways() []string {
	var ret []string
	for _, name := range list.Keys() {
		ret = append(ret, name)
	}
	return ret
}
