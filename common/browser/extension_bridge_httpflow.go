package browser

import "github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"

// BrowserInstance returns the identity advertised by this authenticated Bridge
// connection. Agent HTTP tests and MITM ingresses use the same flow label.
func (c ExtensionBridgeConnection) BrowserInstance() httpctx.BrowserInstance {
	if c.ManagedInstance == nil {
		return httpctx.BrowserInstance{}
	}
	return httpctx.BrowserInstance{
		Manager:        c.ManagedInstance.Manager,
		InstanceID:     c.ManagedInstance.InstanceID,
		InstallationID: c.InstallationID,
		Badge:          c.ManagedInstance.Badge,
	}
}
