package pcaputil

import "bytes"

// GTC is a bounded observation of the peer's token bytes. These fields do not
// establish a protected tunnel, an authenticated server or a token outcome.
func radiusGTCFields(inner map[string]any, code byte, data []byte, note func(string, bool)) {
	inner["Type Name"] = "GTC"
	note("eap-gtc-context-not-verified", false)
	if code == 1 {
		inner["Prompt"] = bytes.Clone(data)
		if len(data) == 0 {
			note("eap-gtc-request-empty", true)
		}
		if len(data) > 0 && data[len(data)-1] == 0 {
			note("eap-gtc-request-null-terminated", true)
		}
		return
	}
	// RFC3748 places the nonempty/NUL-termination constraint on Request,
	// not on Response. Preserve opaque token octets without guessing a secret.
	if len(data) == 0 {
		note("eap-gtc-response-empty", false)
	}
	inner["Token Data"] = bytes.Clone(data)
	inner["Token Bytes"] = len(data)
}
