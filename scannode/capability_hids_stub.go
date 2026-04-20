//go:build !hids

package scannode

import "context"

type hidsCapabilityHooksStub struct{}

func newCapabilityHIDSHooks() capabilityHIDSHooks {
	return hidsCapabilityHooksStub{}
}

func (hidsCapabilityHooksStub) Apply(
	_ *CapabilityManager,
	_ capabilityHIDSApplyInput,
) (CapabilityApplyResult, error) {
	return CapabilityApplyResult{}, ErrHIDSCapabilityNotCompiled
}

func (hidsCapabilityHooksStub) Alerts() <-chan CapabilityRuntimeAlert {
	return nil
}

func (hidsCapabilityHooksStub) CurrentStatus() (CapabilityRuntimeStatus, bool) {
	return CapabilityRuntimeStatus{}, false
}

func (hidsCapabilityHooksStub) OnSessionReady(context.Context) error {
	return nil
}

func (hidsCapabilityHooksStub) Close() error {
	return nil
}
