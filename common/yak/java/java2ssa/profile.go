//go:build !no_language
// +build !no_language

package java2ssa

import (
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var (
	initTime = time.Now()

	lastAnnotationSecond int64
	annotationCostNano   int64 = 0

	lastPackageSecond int64
	packageCostNano   int64 = 0

	lastAssignVariableSecond int64
	assignVariableCostNano   int64 = 0
)

func deltaAssignVariableCostFrom(t time.Time) {
	du := atomic.AddInt64(&assignVariableCostNano, time.Now().Sub(t).Nanoseconds())
	if ret := time.Duration(du).Seconds(); ret > float64(lastAssignVariableSecond+1) {
		log.Infof("abnormal deltaAssignVariableCost cost: %v cost-heavy-percent: %.2f%%", time.Duration(du), 100*(float64(du)/float64(time.Since(initTime))))
		lastAssignVariableSecond = int64(ret)
	}
}

func deltaAnnotationCostFrom(t time.Time) {
	du := atomic.AddInt64(&annotationCostNano, time.Now().Sub(t).Nanoseconds())
	if ret := time.Duration(du).Seconds(); ret > float64(lastAnnotationSecond+1) {
		log.Infof("abnormal deltaAnnotationCost cost: %v cost-heavy-percent: %.2f%%", time.Duration(du), 100*(float64(du)/float64(time.Since(initTime))))
		lastAnnotationSecond = int64(ret)
	}
}

func deltaPackageCostFrom(t time.Time) {
	du := atomic.AddInt64(&packageCostNano, time.Now().Sub(t).Nanoseconds())
	if ret := time.Duration(du).Seconds(); ret > float64(lastPackageSecond+1) {
		log.Infof("abnormal deltaPackageCost cost: %v cost-heavy-percent: %.2f%%", time.Duration(du), 100*(float64(du)/float64(time.Since(initTime))))
		lastPackageSecond = int64(ret)
	}
}

func ShowJavaCompilingCost() {
	ret := atomic.LoadInt64(&annotationCostNano)
	if time.Duration(ret).Milliseconds() > 300 {
		log.Infof("Java Annotation cost: %v", time.Duration(ret))
	}

	ret = atomic.LoadInt64(&packageCostNano)
	if time.Duration(ret).Milliseconds() > 300 {
		log.Infof("Java Package cost: %v", time.Duration(ret))
	}
}

func init() {
}

func (y *singleFileBuilder) AssignVariable(variable *ssa.Variable, val ssa.Value) {
	start := time.Now()
	defer func() {
		deltaAssignVariableCostFrom(start)
	}()
	y.FunctionBuilder.AssignVariable(variable, val)
}
