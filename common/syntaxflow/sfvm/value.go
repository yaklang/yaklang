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
	if count, ok := i.(interface {
		Count() int
	}); ok {
		return count.Count()
	}

	count := 0
	i.Recursive(func(vo ValueOperator) error {
		count++
		return nil
	})
	return count
}
