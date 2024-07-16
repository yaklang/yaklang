package java2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"sync/atomic"
	"time"
)

var (
	annotationCostNano int64 = 0
)

func deltaAnnotationCostFrom(t time.Time) {
	atomic.AddInt64(&annotationCostNano, time.Now().Sub(t).Nanoseconds())
}

func ShowJavaCompilingCost() {
	ret := atomic.LoadInt64(&annotationCostNano)
	if time.Duration(ret).Milliseconds() > 300 {
		log.Infof("Java Annotation cost: %v", time.Duration(ret))
	}
}

func init() {
	ssa.RegisterCostCallback(ShowJavaCompilingCost)
}
