package aispec

import "github.com/yaklang/yaklang/common/utils/omap"

var list = omap.NewOrderedMap(map[string]AIGateway{})

func Register(name string, gateway AIGateway) {
	if gateway == nil {
		return
	}
	list.Set(name, gateway)
}

func Lookup(name string) (AIGateway, bool) {
	return list.Get(name)
}
