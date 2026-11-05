package sharkcli

import "github.com/yaklang/yaklang/common/pcapx/pcaputil"

func tlsCiphertextRefusalDisplayEvent(e *pcaputil.ProtocolEvent) *pcaputil.ProtocolEvent {
	if observation, ok := e.NativeTLSCiphertextRefusal(); ok {
		if _, carried := e.NativeWCFTLSCarrier(); carried {
			e = wcfTLSDisplayEvent(e)
		}
		projected := *e
		projected.Protocol, projected.Status, projected.ExpertCode = "tls", "malformed", string(pcaputil.ErrMalformedMessage)
		projected.Completeness = "rejected-record-envelope"
		projected.Error, projected.Session = observation.Error, observation.Fields
		projected.Fields, projected.Structured = nil, nil
		return &projected
	}
	return e
}

// An IPC caption remains display metadata. It does not reconstruct private
// parser admission, decode success, authentication or a physical byte range.
func tlsCiphertextRefusalCaption(e *pcaputil.ProtocolEvent, projected bool) bool {
	if _, ok := e.NativeTLSCiphertextRefusal(); ok {
		return true
	}
	if !projected || e == nil || e.Protocol != "tls" || e.Completeness != "rejected-record-envelope" || e.Status != "malformed" || e.ExpertCode != string(pcaputil.ErrMalformedMessage) || e.Session["Authentication Verified"] != false || e.Session["Content Visibility"] != "encrypted" {
		return false
	}
	version := number(e.Session["Negotiated Version"])
	if version != 0x303 && version != 0x304 {
		return false
	}
	return e.Error == "MalformedMessage: TLS ciphertext exceeds negotiated AEAD plaintext limit" || e.Error == "MalformedMessage: TLS ciphertext shorter than negotiated AEAD envelope" || version == 0x304 && e.Error == "MalformedMessage: TLS 1.3 protected record legacy version must be 0x0303"
}
