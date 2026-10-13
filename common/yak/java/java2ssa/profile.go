package java2ssa

import (
	"os"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// javaCompileProfil32 tracks Java-only experimental compile-cost counters. The
// counters stay in the tree for local profiling, but every call site is gated
// on javaCompileProfileEnabled so a normal compile pays a single atomic load
// and never constructs the timer or touches the counters. Enable with
// SSA_JAVA_COMPILE_PROFILE=1.
var javaCompileProfileEnabled atomic.Bool

func javaCompileProfileOn() bool {
	return javaCompileProfileEnabled.Load()
}

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
	if ret := time.Duration(du).Seconds(); ret > float64(atomic.LoadInt64(&lastAssignVariableSecond)+1) {
		log.Debugf("abnormal deltaAssignVariableCost cost: %v cost-heavy-percent: %.2f%%", time.Duration(du), 100*(float64(du)/float64(time.Since(initTime))))
		atomic.StoreInt64(&lastAssignVariableSecond, int64(ret))
	}
}

func deltaAnnotationCostFrom(t time.Time) {
	du := atomic.AddInt64(&annotationCostNano, time.Now().Sub(t).Nanoseconds())
	if ret := time.Duration(du).Seconds(); ret > float64(atomic.LoadInt64(&lastAnnotationSecond)+1) {
		log.Debugf("abnormal deltaAnnotationCost cost: %v cost-heavy-percent: %.2f%%", time.Duration(du), 100*(float64(du)/float64(time.Since(initTime))))
		atomic.StoreInt64(&lastAnnotationSecond, int64(ret))
	}
}

func deltaPackageCostFrom(t time.Time) {
	du := atomic.AddInt64(&packageCostNano, time.Now().Sub(t).Nanoseconds())
	if ret := time.Duration(du).Seconds(); ret > float64(atomic.LoadInt64(&lastPackageSecond)+1) {
		log.Debugf("abnormal deltaPackageCost cost: %v cost-heavy-percent: %.2f%%", time.Duration(du), 100*(float64(du)/float64(time.Since(initTime))))
		atomic.StoreInt64(&lastPackageSecond, int64(ret))
	}
}

func ShowJavaCompilingCost() {
	ret := atomic.LoadInt64(&annotationCostNano)
	if time.Duration(ret).Milliseconds() > 300 {
		log.Debugf("Java Annotation cost: %v", time.Duration(ret))
	}

	ret = atomic.LoadInt64(&packageCostNano)
	if time.Duration(ret).Milliseconds() > 300 {
		log.Debugf("Java Package cost: %v", time.Duration(ret))
	}
}

func init() {
	if value := os.Getenv("SSA_JAVA_COMPILE_PROFILE"); value != "" && value != "0" {
		javaCompileProfileEnabled.Store(true)
		log.Infof("java2ssa compile profiling enabled by SSA_JAVA_COMPILE_PROFILE=%s", value)
	}
}

func (y *singleFileBuilder) AssignVariable(variable *ssa.Variable, val ssa.Value) {
	if !javaCompileProfileOn() {
		y.FunctionBuilder.AssignVariable(variable, val)
		return
	}
	start := time.Now()
	defer func() {
		deltaAssignVariableCostFrom(start)
	}()
	y.FunctionBuilder.AssignVariable(variable, val)
}
