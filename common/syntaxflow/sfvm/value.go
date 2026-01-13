package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
)

func ValuesLen(i ValueOperator) int {
	if utils.IsNil(i) {
		return 0
	}
	return i.Count()
}
