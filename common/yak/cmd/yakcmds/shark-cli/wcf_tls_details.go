package sharkcli

import (
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// A contributor can receive its native observation after its capture-time
// caption was frozen. Only the parent's retained proof can replace that caption;
// a worker's public DTO cannot establish protocol or record-query admission.
func wcfTLSObservationCaption(events []*pcaputil.ProtocolEvent) string {
	for _, e := range events {
		if _, ok := e.NativeWCFTLSCarrier(); ok {
			if observed := presentEvents(events); strings.HasPrefix(observed.protocol, "TLS") {
				return observed.protocol
			}
			break
		}
	}
	return ""
}

// Only the worker's already projected display DTO uses this annotation. It
// changes a caption, never protocol admission, authentication or native proof.
func projectedWCFTLSUpgrade(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "tls" || len(e.CarrierLayers) != 1 || e.CarrierLayers[0] != "WCF/MC-NMF" {
		return false
	}
	context, ok := e.Session["Observed WCF Carrier"].(map[string]any)
	return ok && context["Carrier"] == "MC-NMF application/ssl-tls upgrade" && number(context["Observed Session Index"]) > 0
}

func wcfTLSDisplayEvent(e *pcaputil.ProtocolEvent) *pcaputil.ProtocolEvent {
	observation, ok := e.NativeWCFTLSCarrier()
	if !ok {
		return e
	}
	projected := *e
	projected.Protocol, projected.Status = "tls", observation.Status
	projected.Summary, projected.Error = observation.Summary, observation.Error
	projected.CarrierLayers = []string{"WCF/MC-NMF"}
	projected.Fields = nil
	projected.Session, _ = e.NativeWCFTLSFields()
	if !e.NativeWCFTLSWireMatches() {
		// Retained metadata remains useful, but cannot authorize slices into
		// a caller-edited record or fabricate a decrypted byte source.
		context, _ := projected.Session["Observed WCF Carrier"].(map[string]any)
		if context == nil {
			context = map[string]any{}
		}
		context["Record Bytes"] = "owned Raw changed; original record highlights and payload view unavailable"
		projected.Session = map[string]any{"Observed WCF Carrier": context}
		projected.Raw = nil
	}
	return &projected
}
