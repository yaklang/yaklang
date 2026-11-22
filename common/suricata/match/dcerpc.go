package match

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

// DCERPCBinding captures one presentation context binding from a DCE/RPC v5
// Bind/AlterContext PDU or a pcaputil ProtocolEvent.
type DCERPCBinding struct {
	UUID         string
	VersionMajor uint16
	VersionMinor uint16
}

const (
	dcerpcPTypeRequest          uint8 = 0
	dcerpcPTypeResponse         uint8 = 2
	dcerpcPTypeBind             uint8 = 11
	dcerpcPTypeBindAck          uint8 = 12
	dcerpcPTypeAlterContext     uint8 = 14
	dcerpcPTypeAlterContextResp uint8 = 15

	dcerpcFlagObjectUUID uint8 = 0x80
)

type parsedDCERPCPDU struct {
	version    uint8 // 4 (UDP connectionless) or 5 (connection-oriented)
	ptype      uint8
	ifaceUUID  string
	ifaceVer   uint16
	hasIface   bool
	opnum      uint16
	hasOpnum   bool
	stubData   []byte
	endOffset  int
	boundItems []dcerpcContextBind
}

type dcerpcContextBind struct {
	contextID uint16
	binding   DCERPCBinding
}

// DecodeDCERPCUUID converts a 16-byte wire DCE/RPC UUID (mixed-endian when
// littleEndian is true, big-endian otherwise) into canonical lowercase
// "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" form.
func DecodeDCERPCUUID(raw []byte, littleEndian bool) (string, bool) {
	if len(raw) != 16 {
		return "", false
	}
	var timeLow uint32
	var timeMid, timeHi uint16
	if littleEndian {
		timeLow = binary.LittleEndian.Uint32(raw[0:4])
		timeMid = binary.LittleEndian.Uint16(raw[4:6])
		timeHi = binary.LittleEndian.Uint16(raw[6:8])
	} else {
		timeLow = binary.BigEndian.Uint32(raw[0:4])
		timeMid = binary.BigEndian.Uint16(raw[4:6])
		timeHi = binary.BigEndian.Uint16(raw[6:8])
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		timeLow,
		timeMid,
		timeHi,
		binary.BigEndian.Uint16(raw[8:10]),
		raw[10:16],
	), true
}

// DecodeDCERPCAbstractSyntaxHex converts pcaputil's hex-encoded 16-byte
// "Abstract Syntax" field (stored in little-endian wire order) into canonical
// UUID form.
func DecodeDCERPCAbstractSyntaxHex(hexStr string) (string, bool) {
	raw, err := hex.DecodeString(strings.TrimSpace(hexStr))
	if err != nil || len(raw) != 16 {
		return "", false
	}
	return DecodeDCERPCUUID(raw, true)
}

// LooksLikeDCERPC reports whether payload contains a structurally valid
// DCE/RPC v4 or v5 PDU (directly or wrapped inside SMB1/SMB2).
func LooksLikeDCERPC(payload []byte) bool {
	return len(parseDCERPCPDUs(payload, nil)) > 0
}

func dcerpcMatcher(c *matchContext) error {
	if c.Rule == nil || c.Rule.ContentRuleConfig == nil {
		return nil
	}
	cfg := c.Rule.ContentRuleConfig.DCERPCConfig
	usesStub := false
	for _, cr := range c.Rule.ContentRuleConfig.ContentRules {
		if cr != nil && cr.Modifier == modifier.DCERPCStubData {
			usesStub = true
			break
		}
	}
	if cfg == nil && !usesStub {
		return nil
	}

	payload := c.GetBuffer(modifier.Default)
	pdus := parseDCERPCPDUs(payload, c.dcerpcBindings)
	if !c.Must(len(pdus) > 0) {
		return nil
	}

	wantsOpnum := cfg != nil && len(cfg.Opnum) > 0
	wantsIface := cfg != nil && cfg.IfaceUUID != ""

	var matchedStubs [][]byte
	matchedAny := false
	maxEnd := 0

	for _, pdu := range pdus {
		if wantsIface {
			ifaceMatch := false
			if pdu.hasIface && strings.EqualFold(pdu.ifaceUUID, cfg.IfaceUUID) {
				if cfg.IfaceVersion == nil || cfg.IfaceVersion.Match(int(pdu.ifaceVer)) {
					ifaceMatch = true
				}
			}
			if !ifaceMatch && len(pdu.boundItems) > 0 {
				for _, item := range pdu.boundItems {
					if strings.EqualFold(item.binding.UUID, cfg.IfaceUUID) {
						if cfg.IfaceVersion == nil || cfg.IfaceVersion.Match(int(item.binding.VersionMajor)) {
							ifaceMatch = true
							break
						}
					}
				}
			}
			if !ifaceMatch {
				continue
			}
		}

		if wantsOpnum {
			if !pdu.hasOpnum {
				continue
			}
			opMatch := false
			for _, nr := range cfg.Opnum {
				if nr != nil && nr.Match(int(pdu.opnum)) {
					opMatch = true
					break
				}
			}
			if cfg.OpnumNegated {
				opMatch = !opMatch
			}
			if !opMatch {
				continue
			}
		}

		if usesStub {
			if len(pdu.stubData) > 0 {
				matchedStubs = append(matchedStubs, pdu.stubData)
				matchedAny = true
				if pdu.endOffset > maxEnd {
					maxEnd = pdu.endOffset
				}
			}
			continue
		}

		matchedAny = true
		if pdu.endOffset > maxEnd {
			maxEnd = pdu.endOffset
		}
	}

	if !c.Must(matchedAny) {
		return nil
	}
	if maxEnd > c.protocolEvidenceEnd {
		c.protocolEvidenceEnd = maxEnd
	}

	if usesStub {
		if len(matchedStubs) == 1 {
			c.SetBuffer(modifier.DCERPCStubData, matchedStubs[0])
		} else {
			total := 0
			for _, s := range matchedStubs {
				total += len(s)
			}
			joined := make([]byte, 0, total)
			for _, s := range matchedStubs {
				joined = append(joined, s...)
			}
			c.SetBuffer(modifier.DCERPCStubData, joined)
		}
	}
	return nil
}

// parseDCERPCPDUs extracts DCE/RPC v4 (UDP) and v5 (TCP / SMB1 / SMB2) PDUs
// from raw, propagating presentation context bindings across PDUs in order.
func parseDCERPCPDUs(raw []byte, initialBindings map[uint16]DCERPCBinding) []parsedDCERPCPDU {
	if len(raw) < 16 {
		return nil
	}
	bindings := make(map[uint16]DCERPCBinding, len(initialBindings)+2)
	for k, v := range initialBindings {
		bindings[k] = v
	}

	// Fast path 1: raw DCE/RPC v4 (UDP connectionless, 80-byte header).
	if pdu, ok := parseDCERPCv4(raw); ok {
		return []parsedDCERPCPDU{pdu}
	}

	// Fast path 2: raw DCE/RPC v5 stream starting at offset 0.
	if pdus, consumed := parseDCERPCv5Stream(raw, bindings); len(pdus) > 0 && consumed > 0 {
		return pdus
	}

	// Path 3: SMB1 / SMB2 / NetBIOS-wrapped DCE/RPC v5 frames.
	return scanEmbeddedDCERPCv5(raw, bindings)
}

func parseDCERPCv4(raw []byte) (parsedDCERPCPDU, bool) {
	if len(raw) < 80 || raw[0] != 4 {
		return parsedDCERPCPDU{}, false
	}
	ptype := raw[1]
	// C706 connectionless ptype values are 0..10.
	if ptype > 10 {
		return parsedDCERPCPDU{}, false
	}
	littleEndian := (raw[4] & 0x10) != 0
	uuid, ok := DecodeDCERPCUUID(raw[24:40], littleEndian)
	if !ok {
		return parsedDCERPCPDU{}, false
	}
	var ifVer uint32
	var opnum uint16
	var bodyLen uint16
	if littleEndian {
		ifVer = binary.LittleEndian.Uint32(raw[60:64])
		opnum = binary.LittleEndian.Uint16(raw[68:70])
		bodyLen = binary.LittleEndian.Uint16(raw[74:76])
	} else {
		ifVer = binary.BigEndian.Uint32(raw[60:64])
		opnum = binary.BigEndian.Uint16(raw[68:70])
		bodyLen = binary.BigEndian.Uint16(raw[74:76])
	}
	stubEnd := 80 + int(bodyLen)
	if stubEnd > len(raw) {
		stubEnd = len(raw)
	}
	var stub []byte
	if stubEnd > 80 {
		stub = raw[80:stubEnd]
	}
	return parsedDCERPCPDU{
		version:   4,
		ptype:     ptype,
		ifaceUUID: uuid,
		ifaceVer:  uint16(ifVer & 0xffff),
		hasIface:  true,
		opnum:     opnum,
		hasOpnum:  ptype == dcerpcPTypeRequest || ptype == dcerpcPTypeResponse,
		stubData:  stub,
		endOffset: stubEnd,
	}, true
}

func parseDCERPCv5Stream(raw []byte, bindings map[uint16]DCERPCBinding) ([]parsedDCERPCPDU, int) {
	var out []parsedDCERPCPDU
	off := 0
	for off+16 <= len(raw) {
		pdu, fragLen, ok := parseOneDCERPCv5(raw[off:], bindings)
		if !ok || fragLen < 16 {
			break
		}
		pdu.endOffset = off + fragLen
		out = append(out, pdu)
		off += fragLen
	}
	return out, off
}

func parseOneDCERPCv5(raw []byte, bindings map[uint16]DCERPCBinding) (parsedDCERPCPDU, int, bool) {
	if len(raw) < 16 || raw[0] != 5 || (raw[1] != 0 && raw[1] != 1) {
		return parsedDCERPCPDU{}, 0, false
	}
	ptype := raw[2]
	if ptype > 20 {
		return parsedDCERPCPDU{}, 0, false
	}
	flags := raw[3]
	// Data representation: byte 4 integer/char encoding (0x10 = LE, 0x00 = BE),
	// byte 5 float (0=IEEE, 1=VAX, 2=Cray, 3=IBM), bytes 6..7 reserved (0).
	if (raw[4]&0x0f) > 1 || raw[5] > 3 || raw[6] != 0 || raw[7] != 0 {
		return parsedDCERPCPDU{}, 0, false
	}
	littleEndian := (raw[4] & 0x10) != 0
	var fragLen, authLen uint16
	if littleEndian {
		fragLen = binary.LittleEndian.Uint16(raw[8:10])
		authLen = binary.LittleEndian.Uint16(raw[10:12])
	} else {
		fragLen = binary.BigEndian.Uint16(raw[8:10])
		authLen = binary.BigEndian.Uint16(raw[10:12])
	}
	if fragLen < 16 || int(fragLen) > len(raw) {
		return parsedDCERPCPDU{}, 0, false
	}
	if int(authLen) > int(fragLen)-16 {
		return parsedDCERPCPDU{}, 0, false
	}
	pduBytes := raw[:fragLen]
	bodyEnd := int(fragLen)
	if authLen > 0 {
		// Auth verifier header (8 bytes) + auth_value (authLen bytes).
		if bodyEnd >= int(authLen)+8 {
			bodyEnd -= int(authLen) + 8
		}
	}

	pdu := parsedDCERPCPDU{
		version: 5,
		ptype:   ptype,
	}

	switch ptype {
	case dcerpcPTypeBind, dcerpcPTypeAlterContext:
		// Body: max_xmit(2) + max_recv(2) + assoc_group(4) + n_context_elem(1) + reserved(3) = 12 bytes -> offset 28.
		if bodyEnd < 28 {
			return parsedDCERPCPDU{}, 0, false
		}
		nCtx := int(pduBytes[24])
		pos := 28
		for i := 0; i < nCtx && pos+24 <= bodyEnd; i++ {
			var ctxID uint16
			if littleEndian {
				ctxID = binary.LittleEndian.Uint16(pduBytes[pos : pos+2])
			} else {
				ctxID = binary.BigEndian.Uint16(pduBytes[pos : pos+2])
			}
			nTrans := int(pduBytes[pos+2])
			pos += 4
			if pos+20 > bodyEnd {
				break
			}
			uuid, ok := DecodeDCERPCUUID(pduBytes[pos:pos+16], littleEndian)
			var major, minor uint16
			if littleEndian {
				major = binary.LittleEndian.Uint16(pduBytes[pos+16 : pos+18])
				minor = binary.LittleEndian.Uint16(pduBytes[pos+18 : pos+20])
			} else {
				major = binary.BigEndian.Uint16(pduBytes[pos+16 : pos+18])
				minor = binary.BigEndian.Uint16(pduBytes[pos+18 : pos+20])
			}
			pos += 20
			pos += nTrans * 20
			if ok {
				b := DCERPCBinding{UUID: uuid, VersionMajor: major, VersionMinor: minor}
				bindings[ctxID] = b
				pdu.boundItems = append(pdu.boundItems, dcerpcContextBind{contextID: ctxID, binding: b})
				if !pdu.hasIface {
					pdu.ifaceUUID = uuid
					pdu.ifaceVer = major
					pdu.hasIface = true
				}
			}
		}
		if len(pdu.boundItems) == 0 && nCtx > 0 {
			return parsedDCERPCPDU{}, 0, false
		}
	case dcerpcPTypeRequest:
		// Body: alloc_hint(4) + p_cont_id(2) + opnum(2) = 8 bytes -> stub starts at 24 (or 40 if PFC_OBJECT_UUID).
		if bodyEnd < 24 {
			return parsedDCERPCPDU{}, 0, false
		}
		var ctxID, opnum uint16
		if littleEndian {
			ctxID = binary.LittleEndian.Uint16(pduBytes[20:22])
			opnum = binary.LittleEndian.Uint16(pduBytes[22:24])
		} else {
			ctxID = binary.BigEndian.Uint16(pduBytes[20:22])
			opnum = binary.BigEndian.Uint16(pduBytes[22:24])
		}
		stubStart := 24
		if flags&dcerpcFlagObjectUUID != 0 {
			stubStart = 40
		}
		pdu.opnum = opnum
		pdu.hasOpnum = true
		if b, ok := bindings[ctxID]; ok {
			pdu.ifaceUUID = b.UUID
			pdu.ifaceVer = b.VersionMajor
			pdu.hasIface = true
		}
		if stubStart <= bodyEnd {
			pdu.stubData = pduBytes[stubStart:bodyEnd]
		}
	case dcerpcPTypeResponse:
		// Body: alloc_hint(4) + p_cont_id(2) + cancel_count(1) + reserved(1) = 8 bytes -> stub starts at 24.
		if bodyEnd < 24 {
			return parsedDCERPCPDU{}, 0, false
		}
		var ctxID uint16
		if littleEndian {
			ctxID = binary.LittleEndian.Uint16(pduBytes[20:22])
		} else {
			ctxID = binary.BigEndian.Uint16(pduBytes[20:22])
		}
		if b, ok := bindings[ctxID]; ok {
			pdu.ifaceUUID = b.UUID
			pdu.ifaceVer = b.VersionMajor
			pdu.hasIface = true
		}
		if 24 <= bodyEnd {
			pdu.stubData = pduBytes[24:bodyEnd]
		}
	}

	return pdu, int(fragLen), true
}

// scanEmbeddedDCERPCv5 locates DCE/RPC v5 PDUs carried inside SMB1 (\xffSMB),
// SMB2 (\xfeSMB), or NetBIOS Session Service frames across a TCP stream.
func scanEmbeddedDCERPCv5(raw []byte, bindings map[uint16]DCERPCBinding) []parsedDCERPCPDU {
	// Quick gate: only scan embedded offsets if SMB1, SMB2, or a DCE/RPC v5
	// header signature is present in the buffer.
	hasSMB := bytes.Contains(raw, []byte{0xff, 'S', 'M', 'B'}) || bytes.Contains(raw, []byte{0xfe, 'S', 'M', 'B'})
	if !hasSMB && !bytes.Contains(raw, []byte{0x05, 0x00}) {
		return nil
	}

	var out []parsedDCERPCPDU
	for i := 0; i+16 <= len(raw); {
		if raw[i] != 5 || (raw[i+1] != 0 && raw[i+1] != 1) {
			next := bytes.IndexByte(raw[i+1:], 5)
			if next < 0 {
				break
			}
			i += 1 + next
			continue
		}
		pdu, fragLen, ok := parseOneDCERPCv5(raw[i:], bindings)
		if !ok {
			i++
			continue
		}
		// Require concrete DCE/RPC structure (Bind with valid context, or
		// Request/Response with known binding or explicit stub) to avoid
		// matching coincidental 05 00 bytes inside arbitrary SMB data.
		if pdu.ptype == dcerpcPTypeBind || pdu.ptype == dcerpcPTypeAlterContext ||
			pdu.ptype == dcerpcPTypeRequest || pdu.ptype == dcerpcPTypeResponse {
			pdu.endOffset = i + fragLen
			out = append(out, pdu)
			i += fragLen
			continue
		}
		i++
	}
	return out
}
