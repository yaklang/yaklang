package sharkcli

import "fmt"

// A passive observer's missing encoder bytes do not establish that the peer
// exceeded its advertised blocked-stream limit. Keep the pending field state
// visible before packet numbers and ordinary frame summaries can clip it.
func quicQPACKSummary(v map[string]any) string {
	if v["Authentication Verified"] != true {
		return ""
	}
	if reason := textValue(v["QPACK Field Context"]); reason != "" {
		return "QPACK fields pending: " + reason
	}
	if v["QPACK Blocked"] == true {
		return "QPACK fields pending: encoder insertions missing from capture"
	}
	if reason := textValue(v["QPACK Decoder Context"]); reason != "" {
		return "QPACK encoder pending: " + reason
	}
	if rows := mapList(v["QPACK Resumed Streams"]); len(rows) > 0 {
		return quicQPACKRecoveredSummary(rows)
	}
	if rows := mapList(v["QPACK Resumed Field Contexts"]); len(rows) > 0 {
		return fmt.Sprintf("QPACK rechecked %d retained field context(s)", len(rows))
	}
	if rows := mapList(v["QPACK Resumed Encoders"]); len(rows) > 0 {
		return fmt.Sprintf("QPACK rechecked %d retained encoder(s)", len(rows))
	}
	return ""
}

func quicQPACKRecoveredSummary(rows []map[string]any) string {
	streams := make(map[uint64]struct{}, len(rows))
	sections, complete := 0, true
	for _, row := range rows {
		id, known := gpeHeaderInteger(row["HTTP3 Stream ID"], 1<<62-1)
		if !known {
			return fmt.Sprintf("QPACK rechecked %d retained stream observation(s)", len(rows))
		}
		streams[id] = struct{}{}
		count := 0
		for _, frame := range mapList(row["HTTP3 Frames"]) {
			if frame["Frame Type"] == "HEADERS" {
				count++
			}
		}
		sections += count
		complete = complete && count > 0
	}
	if len(streams) == len(rows) {
		return fmt.Sprintf("QPACK replayed retained fields on %d stream(s)", len(rows))
	}
	// Each direction of a bidirectional stream can resume separately in one
	// packet. Replayed observations are not distinct transport streams.
	if complete {
		return fmt.Sprintf("QPACK replayed %d field section(s) on %d stream(s)", sections, len(streams))
	}
	return fmt.Sprintf("QPACK rechecked %d retained observation(s) on %d stream(s)", len(rows), len(streams))
}
