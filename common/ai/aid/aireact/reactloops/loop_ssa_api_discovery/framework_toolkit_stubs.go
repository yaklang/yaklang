package loop_ssa_api_discovery

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// MCMSToolkitStub is a placeholder for future MCMS support.
type MCMSToolkitStub struct{}

func (MCMSToolkitStub) ID() string    { return "mcms" }
func (MCMSToolkitStub) Label() string { return "MCMS (planned)" }
func (MCMSToolkitStub) Detect(*Runtime) (float64, []string) {
	return 0, nil
}
func (MCMSToolkitStub) AcquireCredentials(context.Context, aicommon.AIInvokeRuntime, *Runtime) error {
	return nil
}
func (MCMSToolkitStub) ExtractAPIs(*Runtime) (*CombinedAPICatalog, error) {
	return nil, nil
}
func (MCMSToolkitStub) VerifyAPIs(context.Context, aicommon.AIInvokeRuntime, *Runtime, *CombinedAPICatalog) (*ToolkitVerifyReport, error) {
	return nil, nil
}
func (MCMSToolkitStub) WriteGateArtifacts(*Runtime, *CombinedAPICatalog, *ToolkitVerifyReport) error {
	return nil
}

// OfbizToolkitStub is a placeholder for future Apache OFBiz support.
type OfbizToolkitStub struct{}

func (OfbizToolkitStub) ID() string    { return "ofbiz" }
func (OfbizToolkitStub) Label() string { return "Apache OFBiz (planned)" }
func (OfbizToolkitStub) Detect(*Runtime) (float64, []string) {
	return 0, nil
}
func (OfbizToolkitStub) AcquireCredentials(context.Context, aicommon.AIInvokeRuntime, *Runtime) error {
	return nil
}
func (OfbizToolkitStub) ExtractAPIs(*Runtime) (*CombinedAPICatalog, error) {
	return nil, nil
}
func (OfbizToolkitStub) VerifyAPIs(context.Context, aicommon.AIInvokeRuntime, *Runtime, *CombinedAPICatalog) (*ToolkitVerifyReport, error) {
	return nil, nil
}
func (OfbizToolkitStub) WriteGateArtifacts(*Runtime, *CombinedAPICatalog, *ToolkitVerifyReport) error {
	return nil
}
