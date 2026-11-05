package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// probeSMB1Negotiate admits only the complete, bounded request profile shared
// with application-layer/cifs.yaml. It does not select a dialect or equate the
// legacy MID with an SMB2 MessageId. The field decoder remains the canonical
// CIFS parser; this gate only checks the first record's framing and admission.
func probeSMB1Negotiate(w []byte, limit int) ProbeResult {
	reject := ProbeResult{Verdict: ProbeReject}
	if len(w) < 8 || w[0] != 0 || binary.LittleEndian.Uint32(w[4:8]) != 0x424d53ff {
		return reject
	}
	n := smb2DirectLength(w)
	total := n + 4
	if n < 37 || total > 65574 || total > limit {
		return reject
	}
	if len(w) > 8 && w[8] != 0x72 || len(w) > 13 && w[13]&0x80 != 0 || len(w) > 36 && w[36] != 0 {
		return reject
	}
	if len(w) < 39 {
		return probeNeed("smb", "1-negotiate", len(w), 39)
	}
	count := int(binary.LittleEndian.Uint16(w[37:39]))
	if count < 2 || count != n-35 {
		return reject
	}
	if len(w) < total {
		return probeNeed("smb", "1-negotiate", len(w), total)
	}
	for at, elements := 39, 0; at < total; elements++ {
		if elements >= 1024 || w[at] != 2 {
			return reject
		}
		end := bytes.IndexByte(w[at+1:total], 0)
		if end < 0 {
			return reject
		}
		at += end + 2
	}
	return probeAccept("smb", "1-negotiate", 99)
}
