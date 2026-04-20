//go:build hids && !linux

package scannode

import "context"

type hidsCapabilityHooksUnsupportedPlatform struct{}

func newCapabilityHIDSHooks() capabilityHIDSHooks {
	return hidsCapabilityHooksUnsupportedPlatform{}
}

func (hidsCapabilityHooksUnsupportedPlatform) Apply(
	_ *CapabilityManager,
	_ capabilityHIDSApplyInput,
) (CapabilityApplyResult, error) {
	return CapabilityApplyResult{}, ErrHIDSCapabilityUnsupportedPlatform
}

func (hidsCapabilityHooksUnsupportedPlatform) Alerts() <-chan CapabilityRuntimeAlert {
	return nil
}

func (hidsCapabilityHooksUnsupportedPlatform) CurrentStatus() (CapabilityRuntimeStatus, bool) {
	return CapabilityRuntimeStatus{}, false
}

func (hidsCapabilityHooksUnsupportedPlatform) OnSessionReady(context.Context) error {
	return nil
}

func (hidsCapabilityHooksUnsupportedPlatform) Close() error {
	return nil
}
