package pcaputil

import (
	"bytes"
	"errors"
	"sort"
)

type h3FieldSettingsContextError struct {
	cause      error
	remembered bool
}

func (e *h3FieldSettingsContextError) Error() string { return e.cause.Error() }
func (e *h3FieldSettingsContextError) Unwrap() error { return e.cause }
func isH3FieldSettingsContextError(err error) bool {
	var e *h3FieldSettingsContextError
	return errors.As(err, &e)
}

// A complete HEADERS payload must contain both complete prefix integers even
// when the peer maximum is missing. Capacity-dependent RIC/base semantics and
// dynamic references are opaque until context exists; a future SETTINGS value
// is not qualified remembered SETTINGS for an observed 0-RTT field prefix.
func (q *binQUIC) nativeQPACKFieldContext(dir int, payload []byte, early bool) error {
	encoded, n, err := qpackPrefixed(payload, 8)
	if err != nil {
		return protocolError(ErrMalformedMessage, "qpack: incomplete Required Insert Count in complete field section")
	}
	if n >= len(payload) {
		return protocolError(ErrMalformedMessage, "qpack: missing Base in complete field section")
	}
	if _, _, err = qpackPrefixed(payload[n:], 7); err != nil {
		return protocolError(ErrMalformedMessage, "qpack: incomplete Base in complete field section")
	}
	if encoded == 0 {
		return nil
	}
	if early {
		return &h3FieldSettingsContextError{protocolError(ErrContextRequired, "qpack: 0-RTT dynamic field prefix requires qualified remembered peer SETTINGS"), true}
	}
	if q.qpack.maxKnown[dir] && q.qpack.maxCap[dir]/32 == 0 {
		return protocolError(ErrMalformedMessage, "qpack: dynamic Required Insert Count with observed zero maximum entries")
	}
	if !q.qpack.maxKnown[dir] && q.qpack.maxCap[dir] == 0 && q.qpack.table[dir].capacity == 0 {
		return &h3FieldSettingsContextError{protocolError(ErrContextRequired, "qpack: dynamic field section requires observed peer maximum capacity"), false}
	}
	return nil
}

func (q *binQUIC) waitQPACKFieldSettings(st *quicStream, dir int, sid uint64, fr, info map[string]any, max int, err error) error {
	h3 := st.h3
	if !h3.settingsWaiting[dir] {
		pending := 0
		for _, s := range q.streams {
			if s.h3 != nil && s.h3.settingsWaiting[dir] {
				pending++
			}
		}
		if pending >= max {
			return protocolError(ErrResourceExceeded, "QPACK observer field-context queue limit")
		}
		h3.blockedPDU[dir] = q.eventID
	}
	h3.settingsWaiting[dir], h3.blocked[dir] = true, true
	reason := "peer SETTINGS not observed"
	var missing *h3FieldSettingsContextError
	if errors.As(err, &missing) && missing.remembered {
		h3.rememberedWaiting[dir] = true
		reason = "0-RTT remembered SETTINGS unconfirmed; current SETTINGS cannot qualify its dynamic field prefix"
	}
	info["HTTP3"], fr["HTTP3"] = true, true
	info["HTTP3 Stream ID"], fr["HTTP3 Stream ID"] = sid, sid
	info["Protocol Transition"] = "quic->http3"
	info["QPACK Field Context"], fr["QPACK Field Context"] = reason, reason
	info["QPACK Field Context Scope"] = "bounded current-owner bytes; no cross-connection table or remembered-setting qualification"
	return err
}

// Retry retained field-section bytes only. Packet numbers and STREAM ranges
// were already admitted. Earlier 0-RTT prefixes remain opaque without a native
// remembered-settings provider; do not interpret them using later SETTINGS.
func (q *binQUIC) resumeSettingsFields(dir, max int, info map[string]any) error {
	if !q.native || q.closed || q.retired || !q.qpack.maxKnown[dir] {
		return nil
	}
	var ids []uint64
	for sid, s := range q.streams {
		if s.h3 != nil && s.h3.settingsWaiting[dir] && !s.h3.rememberedWaiting[dir] {
			ids = append(ids, sid)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var rows []map[string]any
	var resource error
	for _, sid := range ids {
		s := q.streams[sid]
		h3 := s.h3
		start := h3.parsed[dir]
		original := h3.blockedPDU[dir]
		sources := quicStreamSources{ranges: append([]quicSourceRange(nil), s.sources[dir].ranges...)}
		encoded := bytes.Clone(h3.buf[dir][start:])
		h3.settingsWaiting[dir], h3.blocked[dir] = false, false
		frame, result := map[string]any{}, map[string]any{}
		err := q.feedHTTP3(dir, sid, uint64(len(h3.buf[dir])), nil, h3.fin[dir], max, frame, result)
		if h3.blocked[dir] {
			h3.blockedPDU[dir] = original
		}
		row := map[string]any{"HTTP3 Stream ID": sid, "Direction": uint64(dir), "Reprocessed From PDU": original, "Reprocessed In PDU": q.eventID, "Field Section Frame Bytes": encoded, "Byte Source": sources.describe(sid, dir, uint64(start), uint64(len(h3.buf[dir]))), "Observation Scope": "original current-owner field bytes; later SETTINGS is context, not a contributor"}
		if fact, ok := result["QPACK Field Error"]; ok {
			row["QPACK Field Error"] = fact
			if info["QPACK Field Error"] == nil {
				info["QPACK Field Error"] = fact
			}
		}
		if err != nil {
			status, typed := classifySessionError(err)
			row["Status"], row["Error"] = status, err.Error()
			if typed != nil {
				row["Expert Code"] = string(typed.Kind)
				if typed.Kind == ErrResourceExceeded {
					resource = err
				}
			}
		} else if h3.blocked[dir] {
			row["Status"] = "context-required"
			row["QPACK Field Context"] = "required insertions not yet observed"
		} else {
			row["Status"] = "decoded"
			row["HTTP3 Frames"] = result["HTTP3 Frames"]
			row["Headers"] = result["Headers"]
		}
		rows = append(rows, row)
		if resource != nil {
			break
		}
	}
	if len(rows) > 0 {
		existing, _ := info["QPACK Resumed Field Contexts"].([]map[string]any)
		info["QPACK Resumed Field Contexts"] = append(existing, rows...)
	}
	return resource
}
