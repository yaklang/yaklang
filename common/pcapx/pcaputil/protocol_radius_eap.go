package pcaputil

import (
	"bytes"
	"encoding/binary"
	"unicode/utf8"
)

// EAP-Message is assembled only from attributes of this one RADIUS message.
// MAC bytes are observed, never verified or treated as an access decision.
func radiusEAPFields(out map[string]any, max int) (bool, error) {
	attrs := out["Attributes"].([]map[string]any)
	code := out["Code"].(int)
	wire := make([]byte, 0)
	var mac []byte
	fragments, auths := 0, 0
	gap, ended := false, false
	empty := false
	badMAC := false
	var fatal, info []string
	note := func(v string, bad bool) {
		if bad {
			fatal = append(fatal, v)
		} else {
			info = append(info, v)
		}
	}
	for _, a := range attrs {
		typ := a["Type"].(byte)
		v := a["Raw"].([]byte)
		if typ == 79 {
			if ended {
				gap = true
			}
			fragments++
			empty = empty || len(v) == 0
			wire = append(wire, v...)
		} else if fragments > 0 {
			ended = true
		}
		if typ == 80 {
			auths++
			if len(v) != 16 {
				badMAC = true
			} else {
				mac = v
			}
		}
	}
	if fragments == 0 && auths == 0 {
		return false, nil
	}
	if code == 4 {
		for _, a := range attrs {
			switch a["Type"].(byte) {
			case 79:
				fatal = append(fatal, "eap-accounting-forbidden-attribute-79")
			case 80:
				fatal = append(fatal, "eap-accounting-forbidden-attribute-80")
			}
		}
		out["EAP Diagnostics"] = fatal
		return true, protocolError(ErrMalformedMessage, "Accounting forbids EAP attributes")
	}
	if fragments > 0 {
		if auths == 0 {
			note("eap-message-authenticator-missing", true)
		}
		for _, a := range attrs {
			switch a["Type"].(byte) {
			case 2:
				note("eap-forbidden-attribute-2", true)
			case 3:
				note("eap-forbidden-attribute-3", true)
			case 18:
				note("eap-forbidden-attribute-18", true)
			case 60:
				note("eap-forbidden-attribute-60", true)
			case 70:
				note("eap-forbidden-attribute-70", true)
			case 75:
				note("eap-forbidden-attribute-75", true)
			}
		}
		if gap && (code == 1 || code == 11) {
			note("eap-attribute-order", true)
		}
	}
	if auths > 1 {
		note("eap-message-authenticator-duplicate", true)
	}
	if badMAC {
		note("eap-message-authenticator-width", true)
	}
	if auths == 1 && !badMAC {
		out["Message-Authenticator"] = bytes.Clone(mac)
	}
	if fragments > 0 {
		inner := map[string]any{"Raw": bytes.Clone(wire), "Fragments": fragments, "Padding Bytes": 0}
		out["EAP"] = inner
		if len(wire) == 0 {
			inner["Start"] = true
			if fragments != 1 {
				note("eap-empty-fragment", true)
			}
			if code != 1 {
				note("eap-outer-start", false)
			}
		} else {
			if empty {
				note("eap-empty-fragment", true)
			}
			if len(wire) < 4 {
				return false, protocolError(ErrMalformedMessage, "EAP truncated header")
			}
			n := int(binary.BigEndian.Uint16(wire[2:4]))
			if n < 4 || n > len(wire) {
				return false, protocolError(ErrMalformedMessage, "EAP declared length")
			}
			ec, id := wire[0], wire[1]
			inner["Code"], inner["Identifier"], inner["Length"], inner["Padding Bytes"] = ec, id, uint16(n), len(wire)-n
			switch ec {
			case 1, 2:
				if n < 5 {
					return false, protocolError(ErrMalformedMessage, "EAP Request/Response missing Type")
				}
				typ, data := wire[4], bytes.Clone(wire[5:n])
				inner["Type"], inner["Type Data"] = typ, data
				switch typ {
				case 1:
					inner["Type Name"] = "Identity"
					if ec == 1 {
						prompt := data
						if at := bytes.IndexByte(prompt, 0); at >= 0 {
							prompt = prompt[:at]
						}
						inner["Prompt"] = bytes.Clone(prompt)
					} else {
						inner["Identity"] = bytes.Clone(data)
						if len(data) > 0 && data[len(data)-1] == 0 {
							note("eap-identity-response-null-terminated", true)
						}
					}
				case 2:
					inner["Type Name"] = "Notification"
					if ec == 1 {
						inner["Notification"] = bytes.Clone(data)
						if len(data) == 0 {
							note("eap-notification-request-empty", true)
						} else if data[len(data)-1] == 0 {
							note("eap-notification-request-null-terminated", true)
						}
						if !utf8.Valid(data) {
							note("eap-notification-request-utf8", true)
						}
					} else {
						inner["Acknowledgement"] = true
						if len(data) != 0 {
							note("eap-notification-response-data", true)
						}
					}
				case 6:
					radiusGTCFields(inner, ec, data, note)
				case 3, 5, 13, 254:
					var dissected bool
					var err error
					if typ == 13 {
						dissected, err = radiusTLSFields(inner, ec, data, max, note)
					} else if typ == 5 {
						dissected, err = radiusOTPFields(inner, ec, data, max, note)
					} else {
						dissected, err = radiusNAKFields(inner, ec, typ, data, max, note)
					}
					if err != nil {
						if len(fatal)+len(info) > 0 {
							out["EAP Diagnostics"] = append(fatal, info...)
						}
						if dissected && len(fatal) > 0 {
							return true, protocolError(ErrMalformedMessage, "RADIUS/EAP normative constraints: %v", fatal)
						}
						return dissected, err
					}
				case 4:
					inner["Type Name"] = "MD5-Challenge"
					if len(data) < 1 {
						return false, protocolError(ErrMalformedMessage, "EAP MD5 missing Value-Size")
					}
					z := int(data[0])
					if z > len(data)-1 {
						return false, protocolError(ErrMalformedMessage, "EAP MD5 Value-Size exceeds data")
					}
					inner["Value Size"], inner["Value"], inner["Name"] = byte(z), bytes.Clone(data[1:1+z]), bytes.Clone(data[1+z:])
					if z == 0 {
						note("eap-md5-value-empty", true)
					}
					if ec == 2 && z != 16 {
						note("eap-md5-response-value-size", true)
					}
					if 1+z == len(data) {
						note("eap-md5-name-empty", false)
					}
				default:
					out["EAP Diagnostics"] = append(append(fatal, info...), "eap-method-unimplemented")
					if len(fatal) > 0 {
						return true, protocolError(ErrMalformedMessage, "RADIUS/EAP normative constraints: %v", fatal)
					}
					return true, protocolError(ErrUnsupportedFeature, "EAP method %d requires a separate profile", typ)
				}
			case 3, 4:
				if n != 4 {
					note("eap-result-data", true)
				}
				inner["Result"] = "success-observed"
				if ec == 4 {
					inner["Result"] = "failure-observed"
				}
				if code == 2 && ec == 4 || code == 3 && ec == 3 || code == 11 {
					note("eap-outer-result-conflict", false)
				}
			default:
				note("eap-code", true)
			}
		}
	}
	if len(fatal)+len(info) > 0 {
		out["EAP Diagnostics"] = append(fatal, info...)
	}
	if len(fatal) > 0 {
		return true, protocolError(ErrMalformedMessage, "RADIUS/EAP normative constraints: %v", fatal)
	}
	return true, nil
}

// Preserve the selected decoder on a typed refusal without scanning a body
// that a byte or collection budget has already refused.
type radiusEAPFailure struct{ error }

func (e *radiusEAPFailure) Unwrap() error { return e.error }

// Only the complete, supported outer record can scope a semantic refusal.
// An allocation-free bounded TLV walk prevents a budget or field refusal from
// hiding a malformed tail. No byte scanning or inferred resynchronisation.
func radiusCompleteByteFrame(w []byte) bool {
	if len(w) < 20 || len(w) > 4096 || int(binary.BigEndian.Uint16(w[2:4])) != len(w) {
		return false
	}
	switch w[0] {
	case 1, 2, 3, 4, 5, 11:
	default:
		return false
	}
	for at := 20; at < len(w); {
		if len(w)-at < 2 || w[at+1] < 2 || int(w[at+1]) > len(w)-at {
			return false
		}
		at += int(w[at+1])
	}
	return true
}
func radiusRecoverableFrame(w []byte, err *ProtocolError, application bool) bool {
	if err == nil || !radiusCompleteByteFrame(w) {
		return false
	}
	switch err.Kind {
	case ErrResourceExceeded, ErrMalformedMessage:
		return true
	default:
		return application
	}
}
