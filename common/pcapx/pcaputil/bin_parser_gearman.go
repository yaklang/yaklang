package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type binGearman struct{}

func gearmanMessageHeader(w []byte, maxBytes int) (int, uint32, bool, error) {
	if len(w) < 4 || !bytes.Equal(w[:4], []byte{0, 'R', 'E', 'Q'}) && !bytes.Equal(w[:4], []byte{0, 'R', 'E', 'S'}) {
		return 0, 0, false, fmt.Errorf("gearman: invalid magic")
	}
	if len(w) < 12 {
		return 0, 0, false, nil
	}
	kind := binary.BigEndian.Uint32(w[4:8])
	length := binary.BigEndian.Uint32(w[8:12])
	if length > uint32(maxBytes-12) {
		return 0, 0, false, fmt.Errorf("gearman: payload exceeds frame budget")
	}
	response := w[3] == 'S'
	switch kind {
	case 1, 4, 7, 9: // CAN_DO, PRE_SLEEP, SUBMIT_JOB, GRAB_JOB
		if response {
			return 0, 0, false, fmt.Errorf("gearman: request command in response envelope")
		}
	case 6, 8, 10, 11: // NOOP, JOB_CREATED, NO_JOB, JOB_ASSIGN
		if !response {
			return 0, 0, false, fmt.Errorf("gearman: response command in request envelope")
		}
	case 13: // WORK_COMPLETE crosses worker->server and server->submitter.
	default:
		return 0, 0, false, fmt.Errorf("gearman: unsupported message type")
	}
	return 12 + int(length), kind, response, nil
}

func probeGearman(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 4 {
		if bytes.Equal(w, []byte{0, 'R', 'E', 'Q'}[:len(w)]) {
			return probeNeed("gearman", "binary-v1", len(w), 12)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if !bytes.Equal(w[:4], []byte{0, 'R', 'E', 'Q'}) {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 12 {
		return probeNeed("gearman", "binary-v1", len(w), 12)
	}
	_, _, _, err := gearmanMessageHeader(w, DefaultParserBudget().MaxMessageBytes)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("gearman", "binary-v1", 100)
}

func (f *binFlow) frameGearman(w []byte) (int, *binSpec, error) {
	n, _, _, err := gearmanMessageHeader(w, f.a.config.MaxMessageBytes)
	if err != nil {
		return 0, nil, err
	}
	return n, f.a.specs["application-layer.gearman/GearmanMessage"], nil
}

func (s *binGearman) consume(raw []byte, maxBytes int) (map[string]any, error) {
	n, kind, response, err := gearmanMessageHeader(raw, maxBytes)
	if err != nil || n != len(raw) {
		return nil, fmt.Errorf("gearman: incomplete or invalid message")
	}
	payload := raw[12:]
	out := map[string]any{"Type Code": kind, "Role": "request", "Payload Length": len(payload)}
	if response {
		out["Role"] = "response"
	}
	parts := bytes.SplitN(payload, []byte{0}, 3)
	switch kind {
	case 1:
		// CAN_DO carries a plain function name in the wire specification.
		// Some generated fixtures include one terminal NUL; accept that form
		// without requiring it or admitting embedded extra arguments.
		function := bytes.TrimSuffix(payload, []byte{0})
		if len(function) == 0 || bytes.IndexByte(function, 0) >= 0 {
			return nil, fmt.Errorf("gearman: invalid CAN_DO function")
		}
		out["Packet Name"], out["Function"] = "CAN_DO", string(function)
	case 4, 6, 9, 10:
		if len(payload) != 0 {
			return nil, fmt.Errorf("gearman: control message has payload")
		}
		out["Packet Name"] = map[uint32]string{4: "PRE_SLEEP", 6: "NOOP", 9: "GRAB_JOB", 10: "NO_JOB"}[kind]
	case 7:
		if len(parts) != 3 || len(parts[0]) == 0 {
			return nil, fmt.Errorf("gearman: invalid SUBMIT_JOB")
		}
		out["Packet Name"], out["Function"], out["Workload"] = "SUBMIT_JOB", string(parts[0]), bytes.Clone(parts[2])
	case 8:
		// JOB_CREATED likewise has a plain job handle, not a required NUL.
		handle := bytes.TrimSuffix(payload, []byte{0})
		if len(handle) == 0 || bytes.IndexByte(handle, 0) >= 0 {
			return nil, fmt.Errorf("gearman: invalid JOB_CREATED handle")
		}
		out["Packet Name"], out["Job Handle"] = "JOB_CREATED", string(handle)
	case 11:
		if len(parts) != 3 || len(parts[0]) == 0 || len(parts[1]) == 0 {
			return nil, fmt.Errorf("gearman: invalid JOB_ASSIGN")
		}
		out["Packet Name"], out["Job Handle"], out["Function"], out["Workload"] = "JOB_ASSIGN", string(parts[0]), string(parts[1]), bytes.Clone(parts[2])
	case 13:
		handle, result, found := bytes.Cut(payload, []byte{0})
		if !found || len(handle) == 0 {
			return nil, fmt.Errorf("gearman: invalid WORK_COMPLETE")
		}
		out["Packet Name"], out["Job Handle"], out["Result"] = "WORK_COMPLETE", string(handle), bytes.Clone(result)
	}
	return out, nil
}
