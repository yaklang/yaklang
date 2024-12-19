package sfvm

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func AutoValue(i any) ValueOperator {
	log.Warnf("TBD: AutoValue: %v", i)
	return i.(ValueOperator)
}

func ValuesLen(i ValueOperator) int {
	if utils.IsNil(i) {
		return 0
	}
	count := 0
	i.Recursive(func(vo ValueOperator) error {
		count++
		return nil
	})
	return count
}
