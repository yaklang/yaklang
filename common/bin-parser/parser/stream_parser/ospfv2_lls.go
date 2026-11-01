package stream_parser

import (
	"encoding/binary"
	"errors"
	"fmt"
)

func ospfv2LLSAuthDeclarations(r *controlReader, row map[string]any, auth uint64) {
	if row == nil || auth != 2 {
		return
	}
	row["Authentication Verified"], row["Checksum Verified"] = false, false
	row["Checksum Scope"] = "omitted: cryptographic LLS authentication; MAC, key and SA unavailable"
	if checksum, present := row["Checksum"].(uint64); present && checksum != 0 {
		r.err = errors.Join(r.err, fmt.Errorf("%w: cryptographic LLS checksum field must be zero", ErrOSPFv2WireFormat))
	}
}

// LLS begins after the OSPF authentication digest, is measured in 32-bit words
// and has a checksum extent independent of the declared OSPF packet.
func ospfv2LLS(r *controlReader, auth uint64, authentication map[string]any, budget *int) map[string]any {
	start := r.at
	if len(r.w)-start < 4 {
		row := ospfv2Child(r, len(r.w), func(c *controlReader) {
			c.num("Checksum", 2, false)
			c.num("Length Words", 2, false)
		})
		ospfv2LLSAuthDeclarations(r, row, auth)
		return row
	}
	n := int(binary.BigEndian.Uint16(r.w[start+2:])) * 4
	end := min(start+max(4, n), len(r.w))
	row := ospfv2Child(r, end, func(c *controlReader) {
		c.num("Checksum", 2, false)
		c.num("Length Words", 2, false)
		c.need(n >= 4, "OSPFv2 LLS length is below header")
		var tlvs []map[string]any
		seen := map[uint64]bool{}
		for c.err == nil && c.at < len(c.w) && ospfv3Charge(c, budget) {
			a := c.at
			if len(c.w)-a < 4 {
				tlvs = ospfv2KeepRow(tlvs, ospfv2Child(c, len(c.w), func(t *controlReader) {
					t.num("TLV Type", 2, false)
					t.num("Length", 2, false)
				}))
				break
			}
			length := int(binary.BigEndian.Uint16(c.w[a+2:]))
			padded := (length + 3) &^ 3
			declaredEnd := a + 4 + padded
			tlv := ospfv2Child(c, min(declaredEnd, len(c.w)), func(t *controlReader) {
				typ := t.num("TLV Type", 2, false)
				ln := t.num("Length", 2, false)
				switch typ {
				case 1:
					if ln == 4 {
						t.num("Extended Options and Flags", 4, false)
					} else {
						t.take("Opaque Value", int(ln))
						t.need(false, "OSPFv2 LLS Extended Options TLV length must be four")
					}
				case 2:
					t.need(ln >= 4, "OSPFv2 LLS cryptographic TLV lacks sequence")
					if t.err != nil {
						return
					}
					sequence := t.num("Sequence", 4, false)
					sequenceObserved := t.err == nil
					t.take("Authentication Data", int(ln)-4)
					if auth == 2 && sequenceObserved {
						expected, present := authentication["Sequence"].(uint64)
						if present {
							t.info["Sequence Matches OSPF Header"] = sequence == expected
							t.need(sequence == expected, "OSPFv2 LLS sequence differs from observed authentication header")
						}
						if expected, present := authentication["Authentication Data Length"].(uint64); present {
							t.info["Authentication Data Length Matches Header"] = ln-4 == expected
							t.need(ln-4 == expected, "OSPFv2 LLS authentication length differs from observed packet digest length")
						}
					}
					t.need(auth != 0 && auth != 1, "OSPFv2 LLS cryptographic TLV without cryptographic authentication")
					t.need(declaredEnd == start+n, "OSPFv2 LLS cryptographic TLV must be last")
				default:
					t.take("Opaque Value", int(ln))
				}
				if padded > length {
					t.take("Padding", padded-length)
				}
				if typ == 1 || typ == 2 {
					t.need(!seen[typ], "OSPFv2 duplicate singleton LLS TLV")
					seen[typ] = true
				}
			})
			tlvs = ospfv2KeepRow(tlvs, tlv)
			c.need(declaredEnd <= len(c.w), "OSPFv2 LLS TLV exceeds declared block")
		}
		c.info["TLVs"] = tlvs
		if c.err == nil && auth == 2 && start+n <= len(r.w) {
			c.need(seen[2], "OSPFv2 cryptographic LLS block lacks CA TLV")
		}
	})
	if row != nil {
		row["Authentication Verified"] = false
		row["Checksum Verified"] = false
		row["Checksum Scope"] = "unverified: complete LLS block unavailable"
		if auth == 2 {
			row["Checksum Scope"] = "omitted: cryptographic LLS authentication; MAC, key and SA unavailable"
		} else if n >= 4 && start+n <= len(r.w) && (auth == 0 || auth == 1) {
			valid := ospfv2Sum(r.w[start:start+n]) == 0
			row["Checksum Verified"], row["Checksum Scope"] = valid, "LLS block only; independent of OSPF packet checksum"
			if !valid {
				r.err = ospfv2IntegrityFailure(r.err, "LLS block checksum failed")
			}
		} else if auth > 2 {
			row["Checksum Scope"] = "unverified: unselected authentication type"
		}
	}
	r.need(start+n <= len(r.w), "OSPFv2 LLS block exceeds captured carrier")
	if n < 4 || start+n <= len(r.w) {
		r.err = ospfv2CompleteFormatError(r.err)
	}
	ospfv2LLSAuthDeclarations(r, row, auth)
	return row
}
