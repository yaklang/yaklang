package pcaputil

import (
	"encoding/binary"
	"fmt"
)

type binMongo struct {
	pending map[uint32]string
}

func probeMongo(w []byte, limit int) ProbeResult {
	if len(w) < 4 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(binary.LittleEndian.Uint32(w[:4]))
	if n < 16 || n > 1<<20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 16 {
		return probeNeed("mongodb", "op-msg", len(w), 16)
	}
	op := binary.LittleEndian.Uint32(w[12:16])
	if op != 2013 && op != 2012 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("mongodb", "op-msg", 90)
}

func (f *binFlow) frameMongo(w []byte) (int, *binSpec, error) {
	if f.mongo == nil {
		return 0, nil, sessionContext("MongoDB session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(f.mongo.pending))*16); err != nil {
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	n := int(binary.LittleEndian.Uint32(w[:4]))
	if n < 16 {
		return 0, nil, fmt.Errorf("mongodb: message length smaller than header")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	op := binary.LittleEndian.Uint32(w[12:16])
	if op != 2013 && op != 2012 {
		return 0, nil, fmt.Errorf("mongodb: unsupported opcode %d", op)
	}
	return n, f.spec("mongodb_fields", "MongoDBFields"), nil
}

func (m *binMongo) consume(raw []byte, result map[string]any) (map[string]any, error) {
	if len(raw) < 16 {
		return nil, fmt.Errorf("mongodb: truncated header")
	}
	req := binary.LittleEndian.Uint32(raw[4:8])
	respTo := binary.LittleEndian.Uint32(raw[8:12])
	op := binary.LittleEndian.Uint32(raw[12:16])
	info := map[string]any{
		"Request ID":    uint64(req),
		"Response To":   uint64(respTo),
		"Op Code":       uint64(op),
		"Opcode Name":   mongoSessionOpcode(op),
		"Context Level": "observed",
	}
	if meta, ok := result["metadata"].(map[string]any); ok {
		for _, k := range []string{"Opcode Name", "Compressor", "Sequence Identifier", "Section Count", "Inner Opcode Name", "More To Come", "Uncompressed Size"} {
			if v, ok := meta[k]; ok {
				info[k] = v
			}
		}
	}
	if respTo != 0 {
		if _, ok := m.pending[respTo]; ok {
			delete(m.pending, respTo)
			info["Matched Request"] = true
		} else {
			info["Unsolicited"] = true
		}
	} else {
		m.pending[req] = mongoSessionOpcode(op)
		info["Outstanding"] = true
	}
	return info, nil
}

func mongoSessionOpcode(op uint32) string {
	switch op {
	case 2013:
		return "OP_MSG"
	case 2012:
		return "OP_COMPRESSED"
	}
	return "UNKNOWN"
}
