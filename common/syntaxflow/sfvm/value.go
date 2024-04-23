package sfvm

import "github.com/yaklang/yaklang/common/log"

func AutoValue(i any) ValueOperator {
	log.Warnf("TBD: AutoValue: %v", i)
	return i.(ValueOperator)
}
