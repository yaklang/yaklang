package pcaputil

import (
	"bytes"
	"errors"
	"sort"
)

// Missing passive-observer context does not reject authenticated transport
// frames or prevent independent STREAMs in the same packet from being observed.
type h3EncoderContextError struct{ cause error }

func (e *h3EncoderContextError) Error() string { return e.cause.Error() }
func (e *h3EncoderContextError) Unwrap() error { return e.cause }
func isH3EncoderContextError(err error) bool {
	var e *h3EncoderContextError
	return errors.As(err, &e)
}

// SETTINGS supplies decoder context; it is not a new contribution to an older
// encoder stream. Retry only the retained unparsed suffix, without feeding any
// QUIC packet, transport frame, or already applied instruction a second time.
func (q *binQUIC) resumeSettingsEncoders(dir, max int, info map[string]any) error {
	if !q.native || q.retired || q.closed || !q.qpack.maxKnown[dir] {
		return nil
	}
	var ids []uint64
	for sid, stream := range q.streams {
		if stream.h3 != nil && stream.h3.kind == "encoder" && stream.h3.encoderWaiting[dir] && stream.canResumeCurrentQPACKEncoder(dir) {
			ids = append(ids, sid)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var resumed []map[string]any
	var resourceError error
	for _, sid := range ids {
		stream := q.streams[sid]
		h3 := stream.h3
		start := h3.parsed[dir]
		original := h3.encoderWaitPDU[dir]
		encoded := bytes.Clone(h3.buf[dir][start:])
		// feedHTTP3 prunes consumed witnesses in place. Keep an independent
		// range slice so a completed prefix cannot claim the later incomplete
		// suffix's packets, parents or bytes as its contributors.
		sources := quicStreamSources{ranges: append([]quicSourceRange(nil), stream.sources[dir].ranges...)}
		witness := sources.describe(sid, dir, uint64(start), uint64(len(h3.buf[dir])))
		frame, result := map[string]any{}, map[string]any{}
		err := q.feedHTTP3(dir, sid, uint64(len(h3.buf[dir])), nil, h3.fin[dir], max, frame, result)
		item := map[string]any{"HTTP3 Stream ID": sid, "Direction": uint64(dir), "Reprocessed From PDU": original,
			"Reprocessed In PDU": q.eventID, "Instruction Bytes": encoded, "Byte Source": witness,
			"Observation Scope": "retained encoder instruction bytes; later SETTINGS is context, not a byte contributor"}
		if h3.parsed[dir] > start {
			consumed := h3.parsed[dir] - start
			item["Status"] = "decoded"
			item["Instruction Bytes"] = bytes.Clone(encoded[:consumed])
			item["Byte Source"] = sources.describe(sid, dir, uint64(start), uint64(h3.parsed[dir]))
			item["Incomplete Suffix Bytes"] = uint64(len(encoded) - consumed)
			item["QPACK Instructions"] = result["QPACK Instructions"]
			item["QPACK Capacity"] = q.qpack.table[dir].capacity
			item["QPACK Insert Count"] = q.qpack.table[dir].insertCount
			if err != nil {
				item["Dependent Decode Error"] = err.Error()
			}
		} else if err != nil {
			status, typed := classifySessionError(err)
			item["Status"], item["Error"] = status, err.Error()
			if typed != nil {
				item["Expert Code"] = string(typed.Kind)
			}
		} else {
			item["Status"] = "incomplete"
		}
		resumed = append(resumed, item)
		if fact, ok := result["QPACK Field Error"]; ok && info["QPACK Field Error"] == nil {
			info["QPACK Field Error"] = fact
		}
		if headers, ok := result["QPACK Resumed Streams"].([]map[string]any); ok {
			existing, _ := info["QPACK Resumed Streams"].([]map[string]any)
			info["QPACK Resumed Streams"] = append(existing, headers...)
		}
		if _, typed := classifySessionError(err); typed != nil && typed.Kind == ErrResourceExceeded {
			resourceError = err
			break
		}
	}
	if len(resumed) > 0 {
		existing, _ := info["QPACK Resumed Encoders"].([]map[string]any)
		info["QPACK Resumed Encoders"] = append(existing, resumed...)
	}
	return resourceError
}
