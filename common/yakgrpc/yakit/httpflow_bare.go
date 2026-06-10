package yakit

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func httpFlowBareResponseKey(flowID uint) string {
	return strconv.FormatUint(uint64(flowID), 10) + "_response"
}

func httpFlowWireResponse(wirePacket, rspHint []byte) []byte {
	if len(wirePacket) > 0 {
		return wirePacket
	}
	return rspHint
}

func httpFlowShouldStoreBareWire(wire, display []byte, noFixContentLength bool) bool {
	if noFixContentLength || len(wire) == 0 || httpFlowResponsePacketsEqual(wire, display) {
		return false
	}
	return httpFlowAutoFixedCharset(wire, display)
}

// httpFlowAutoFixedCharset detects FixHTTPResponse charset/body conversion (not header-only or replacer edits).
func httpFlowAutoFixedCharset(wire, display []byte) bool {
	_, wireBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(wire)
	_, displayBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(display)
	if bytes.Equal(wireBody, displayBody) {
		return false
	}
	if len(displayBody) > 0 && utf8.Valid(displayBody) && len(wireBody) > 0 && !utf8.Valid(wireBody) {
		return true
	}
	wct := strings.ToLower(lowhttp.GetHTTPPacketHeader(wire, "Content-Type"))
	dct := strings.ToLower(lowhttp.GetHTTPPacketHeader(display, "Content-Type"))
	if wct == dct {
		return false
	}
	return strings.Contains(dct, "charset=utf-8") &&
		(strings.Contains(wct, "charset=gbk") || strings.Contains(wct, "charset=gb2312") || strings.Contains(wct, "charset:gbk"))
}

func resolveHTTPFlowStoredResponse(wire, rspRaw, fixRspRaw []byte, noFixContentLength bool) []byte {
	if len(fixRspRaw) > 0 {
		return fixRspRaw
	}
	if noFixContentLength {
		if len(wire) > 0 {
			return wire
		}
		return rspRaw
	}
	fixSource := wire
	if len(rspRaw) > 0 && len(wire) > 0 && !bytes.Equal(wire, rspRaw) {
		// MITM hijack/replacer, too-large placeholder, etc.: display packet is authoritative.
		fixSource = rspRaw
	} else if len(fixSource) == 0 {
		fixSource = rspRaw
	}
	if len(fixSource) == 0 {
		return nil
	}
	fixed, _, err := lowhttp.FixHTTPResponse(fixSource)
	if err != nil || len(fixed) == 0 {
		return fixSource
	}
	if len(rspRaw) > 0 && bytes.Equal(rspRaw, fixed) {
		return rspRaw
	}
	return fixed
}

func httpFlowResponsePacketsEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	at := truncateHTTPPacketBodyForStorage(a, maxStoredHTTPFlowResponseBodyBytes)
	bt := truncateHTTPPacketBodyForStorage(b, maxStoredHTTPFlowResponseBodyBytes)
	return bytes.Equal(at, bt)
}

func saveHTTPFlowBareResponse(db *gorm.DB, flowID uint, wire []byte) error {
	if db == nil {
		db = consts.GetGormProjectDatabase()
	}
	if flowID == 0 || len(wire) == 0 {
		return nil
	}
	wire = truncateHTTPPacketBodyForStorage(wire, maxStoredHTTPFlowResponseBodyBytes)
	return SetProjectKeyWithGroup(db, httpFlowBareResponseKey(flowID), wire, BARE_RESPONSE_GROUP)
}

func afterSaveHTTPFlowBareResponse(wire []byte) func(*schema.HTTPFlow) {
	wire = bytes.Clone(wire)
	return func(flow *schema.HTTPFlow) {
		if flow == nil || flow.ID == 0 {
			return
		}
		if err := saveHTTPFlowBareResponse(nil, flow.ID, wire); err != nil {
			log.Errorf("save httpflow bare response failed: %s", err)
		}
	}
}

// lowhttpResponsePackets: BareResponse=wire, RawPacket=display (fixed unless NoFix).
func lowhttpResponsePackets(r *lowhttp.LowhttpResponse) (wire, display []byte, noFixContentLength bool) {
	if r == nil {
		return nil, nil, false
	}
	wire = r.BareResponse
	if len(wire) == 0 {
		wire = r.RawPacket
	}
	display = r.RawPacket
	if len(display) == 0 {
		display = r.BareResponse
	}
	noFixContentLength = len(r.BareResponse) > 0 && bytes.Equal(r.RawPacket, r.BareResponse)
	return wire, display, noFixContentLength
}
