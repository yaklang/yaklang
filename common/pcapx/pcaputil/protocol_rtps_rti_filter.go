package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
)

// A selected reported vendor is a layout discriminator, not authentication.
// The multichannel locator layout is kind/address/port, unlike Locator_t.
func rtps20RTIFilterFields(b []byte, o binary.ByteOrder, remaining *int, depth int) (map[string]any, error) {
	if len(b) < 4 {
		return nil, rtpsMalformed("RTI filter channel count is truncated")
	}
	if depth < 6 {
		return nil, protocolError(ErrResourceExceeded, "RTI filter projection depth")
	}
	n := uint64(o.Uint32(b))
	at := 4
	if n > uint64(*remaining) {
		return nil, protocolError(ErrResourceExceeded, "RTI filter channel collection budget")
	}
	if n > 0 && (len(b) < 8 || n > uint64((len(b)-8)/8)) {
		return nil, rtpsMalformed("RTI filter channels cannot fit bounded parameter")
	}
	*remaining -= int(n)
	f := map[string]any{"Interpretation": "unverified-rti-multichannel-filter-list", "Num Channels": uint32(n), "Channels": make([]map[string]any, 0)}
	text := func(out map[string]any, key string) error {
		if len(b)-at < 4 {
			return rtpsMalformed("RTI filter string length is truncated")
		}
		size := uint64(o.Uint32(b[at:]))
		at += 4
		padded := (size + 3) &^ uint64(3)
		if padded > uint64(len(b)-at) {
			return rtpsMalformed("RTI filter string or its padding is truncated")
		}
		out[key+" Length"] = uint32(size)
		out[key+" Bytes"] = bytes.Clone(b[at : at+int(size)])
		out[key+" Padding Bytes"] = bytes.Clone(b[at+int(size) : at+int(padded)])
		at += int(padded)
		return nil
	}
	if n > 0 {
		if depth < 8 {
			return nil, protocolError(ErrResourceExceeded, "RTI filter channel projection depth")
		}
		if err := text(f, "Filter Name"); err != nil {
			return nil, err
		}
		if n > uint64((len(b)-at)/8) {
			return nil, rtpsMalformed("RTI filter channels are truncated after name")
		}
		cs := make([]map[string]any, int(n))
		for i := range cs {
			if len(b)-at < 4 {
				return nil, rtpsMalformed("RTI filter locator count is truncated")
			}
			count := uint64(o.Uint32(b[at:]))
			at += 4
			if count > uint64(*remaining) {
				return nil, protocolError(ErrResourceExceeded, "RTI filter locator collection budget")
			}
			if len(b)-at < 4 || count > uint64((len(b)-at-4)/24) {
				return nil, rtpsMalformed("RTI filter locators or expression length are truncated")
			}
			if count > 0 && depth < 10 {
				return nil, protocolError(ErrResourceExceeded, "RTI multichannel locator projection depth")
			}
			*remaining -= int(count)
			ls := make([]map[string]any, int(count))
			for j := range ls {
				kind := int32(o.Uint32(b[at:]))
				address := b[at+4 : at+20]
				port := o.Uint32(b[at+20:])
				at += 24
				loc := map[string]any{"Kind": kind, "Port": port, "Address Bytes": bytes.Clone(address), "Interpretation": "unselected-multichannel-locator-kind"}
				switch kind {
				case 1:
					loc["Interpretation"] = "reported-multichannel-udp-v4-locator"
					loc["IPv4 Address"] = netip.AddrFrom4([4]byte(address[12:])).String()
					loc["IPv4 Prefix Bytes"] = bytes.Clone(address[:12])
				case 2:
					loc["Interpretation"] = "reported-multichannel-udp-v6-locator"
					loc["IPv6 Address"] = netip.AddrFrom16([16]byte(address)).String()
				}
				ls[j] = loc
			}
			cs[i] = map[string]any{"Num Locators": uint32(count), "Locators": ls}
			if err := text(cs[i], "Filter Expression"); err != nil {
				return nil, err
			}
		}
		f["Channels"] = cs
	}
	if at < len(b) {
		f["Extension Bytes"] = bytes.Clone(b[at:])
	}
	return f, nil
}

// WithSessionRTPS20RTIFilter observes literal proprietary parameter fields.
// Strings remain byte observations; filters are never evaluated or executed.
func WithSessionRTPS20RTIFilter(port uint16) ProtocolSessionOption {
	return func(s *captureSession) error {
		if port == 0 {
			return fmt.Errorf("RTI filter observer port must be nonzero")
		}
		old := s.f.a.datagramDecodeAs[port]
		if old != "" && old != "rtps20-rti-filter" {
			return fmt.Errorf("conflicting RTI filter profile")
		}
		s.f.a.datagramDecodeAs[port] = "rtps20-rti-filter"
		return nil
	}
}
func decodeRTPS20RTIFilter(w []byte, b ParserBudget) (map[string]any, error) {
	return decodeRTPS20Mode(w, b.MaxCollectionElements, b.MaxRecursionDepth, true)
}
func (a *binParser) rtps20RTIFilterSelected(src, dst uint16) bool {
	p := a.datagramDecodeAs[dst]
	if p == "" {
		p = a.datagramDecodeAs[src]
	}
	return p == "rtps20-rti-filter"
}
func (s *captureSession) probeRTPS20RTIFilter(data []byte) (ProbeResult, bool) {
	a := s.f.a
	if !a.rtps20RTIFilterSelected(s.f.ports[0], s.f.ports[1]) {
		return ProbeResult{}, false
	}
	reject := func(err error) (ProbeResult, bool) {
		return ProbeResult{Verdict: ProbeReject, Protocol: "rtps", Reason: err.Error()}, true
	}
	if len(data) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) || 32768+512*int64(len(data)) > int64(a.config.MaxBufferedBytes)-a.buffered.Load() {
		return reject(protocolError(ErrResourceExceeded, "RTI filter byte/projection budget"))
	}
	if _, err := decodeRTPS20RTIFilter(data, a.budget); err != nil {
		return reject(err)
	}
	return probeAccept("rtps", "rtps20-rti-filter-native", 95), true
}
func (a *binParser) decodeRTPS20RTIFilterInput(e *ProtocolEvent, w []byte, inputErr error) bool {
	e.Protocol, e.Profile, e.Admission, e.Completeness = "rtps", "rtps20-rti-filter-native", "explicit-decode-as", "message"
	raw := w
	var fields map[string]any
	var err error
	transient := 32768 + 512*int64(len(w))
	if inputErr != nil {
		err = inputErr
		raw = nil
	} else if len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		err = protocolError(ErrResourceExceeded, "RTI filter byte budget")
		raw = nil
	} else if !a.reserveEvidence(transient) {
		err = protocolError(ErrResourceExceeded, "RTI filter projection budget")
		raw = nil
	} else {
		defer a.buffered.Add(-transient)
		fields, err = decodeRTPS20RTIFilter(w, a.budget)
	}
	if err == nil {
		e.semanticFields = cloneSession(fields)
		e.semanticSession = cloneSession(fields)
		e.Session = cloneSession(fields)
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if raw == nil {
		a.messageBytes.Add(uint64(len(w)))
		if e.Status == "limited" {
			a.limited.Add(uint64(len(w)))
		}
	}
	return true
}
