package ssaapi

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
)

func (v *Value) Backtrack() *omap.OrderedMap[string, *Value] {
	ret := omap.NewOrderedMap[string, *Value](map[string]*Value{})
	var vals = utils.NewStack[*Value]()
	var count = 1
	var current = v
	vals.Push(v)
	for current != nil {
		deps := current.DependOn
		var p *Value
		if len(deps) > 0 {
			p = deps[0]
		} else {
			break
		}
		count++
		vals.Push(p)
		current = p
	}
	for i := 0; i < count; i++ {
		err := ret.Push(vals.Pop())
		if err != nil {
			log.Warn(err)
		}
	}
	return ret
}

func (v *Value) ShowBacktrack() {
	var buf bytes.Buffer
	om := v.Backtrack()
	buf.WriteString("===================== Backtrack from [t" + fmt.Sprint(v.GetId()) + "]`" + v.String() + "` =====================: \n\n")
	if om == nil || om.Len() <= 0 {
		buf.WriteString("empty parent\n")
		fmt.Println(buf.String())
		return
	}

	for index, track := range om.Values() {
		indent := strings.Repeat(" ", index*2) + fmt.Sprintf("[depth:%2d]->", track.GetDepth())
		buf.WriteString(indent + track.String() + "\n")
	}
	fmt.Println(buf.String())
}
