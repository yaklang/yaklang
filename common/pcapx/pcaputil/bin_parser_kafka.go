package pcaputil

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type binKafka struct {
	client  int
	pending map[int32]kafkaPending
	expired map[int32]bool
	clock   time.Time
}

type kafkaPending struct {
	api, version int16
	id           uint64
	ts           time.Time
}

func kafkaSupported(api, ver uint16) bool {
	return stream_parser.KafkaVersionSupported(int16(api), int16(ver))
}

func probeKafka(w []byte, limit int) ProbeResult {
	if len(w) < 8 {
		if len(w) > 0 && w[0] == 0 {
			return probeNeed("kafka", "request", len(w), 8)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	size := int(binary.BigEndian.Uint32(w[:4]))
	if size < 10 || size > 1<<20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	api := binary.BigEndian.Uint16(w[4:6])
	ver := binary.BigEndian.Uint16(w[6:8])
	if !kafkaSupported(api, ver) {
		return ProbeResult{Verdict: ProbeReject}
	}
	// API/version alone also match a server HTTP/2 SETTINGS frame. Inspect
	// the complete fixed request header before committing the connection.
	if len(w) < 14 {
		return probeNeed("kafka", "request", len(w), 14)
	}
	clientLen := int(int16(binary.BigEndian.Uint16(w[12:14])))
	if clientLen < -1 || clientLen > size-10 {
		return ProbeResult{Verdict: ProbeReject}
	}
	// A Produce v0 prefix can still be indistinguishable from initial peer
	// SETTINGS. Leave that frame unclassified until the client preface or
	// more request bytes resolve the ambiguity. Real Kafka requests with
	// this prefix can be admitted once they extend beyond the short H2 frame.
	h2Size := size >> 8
	if w[3] == 4 && w[4] == 0 && binary.BigEndian.Uint32(w[5:9]) == 0 && h2Size%6 == 0 && len(w) <= 9+h2Size {
		return probeNeed("kafka", "ambiguous-settings", len(w), min(limit, 10+h2Size))
	}
	return probeAccept("kafka", kafkaSessionAPIName(int16(api)), 88)
}

func (f *binFlow) frameKafka(dir int, w []byte) (int, *binSpec, error) {
	k := f.kafka
	if k == nil {
		return 0, nil, sessionContext("Kafka session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(k.pending))*64 + int64(len(k.expired))*24); err != nil {
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	size := int(binary.BigEndian.Uint32(w[:4]))
	if size < 4 {
		return 0, nil, fmt.Errorf("kafka: length smaller than correlation header")
	}
	n := 4 + size
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	entry := "KafkaResponseFields"
	if k.client < 0 || dir == k.client {
		entry = "KafkaRequestFields"
	}
	return n, f.spec("kafka_fields", entry), nil
}

func (f *binFlow) consumeKafka(dir int, e *ProtocolEvent, result map[string]any) (map[string]any, error) {
	k := f.kafka
	raw := e.Raw
	if e.Timestamp.After(k.clock) {
		k.clock = e.Timestamp
	}
	for id, p := range k.pending {
		if k.clock.Sub(p.ts) > 30*time.Second {
			delete(k.pending, id)
			if k.expired == nil {
				k.expired = map[int32]bool{}
			}
			if len(k.expired) >= f.a.budget.MaxCollectionElements {
				return nil, protocolError(ErrResourceExceeded, "Kafka expired correlation history")
			}
			k.expired[id] = true
		}
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	if len(raw) < 8 {
		return nil, fmt.Errorf("kafka: truncated message")
	}
	info := map[string]any{"Context Level": "observed"}
	if meta, ok := result["metadata"].(map[string]any); ok {
		for _, key := range []string{"Flexible", "Header Version", "Request Header", "Tagged Fields", "Client Software Name", "Client Software Version", "API Name", "API Key", "API Version", "Correlation ID", "Client ID", "Topic Name", "Magic", "Compression", "Records Count", "Base Offset", "Partition", "Topic Results", "Acks", "Timeout", "Batches"} {
			if v, ok := meta[key]; ok {
				info[key] = v
			}
		}
	}
	request := k.client < 0 || dir == k.client
	if request {
		if len(raw) < 14 {
			return nil, fmt.Errorf("kafka: truncated request header")
		}
		ver := int16(binary.BigEndian.Uint16(raw[6:8]))
		info["API Version"] = ver
		if k.client < 0 {
			k.client = dir
		}
		api := int16(binary.BigEndian.Uint16(raw[4:6]))
		corr := int32(binary.BigEndian.Uint32(raw[8:12]))
		info["API Key"] = api
		info["Correlation ID"] = corr
		info["API Name"] = kafkaSessionAPIName(api)
		if k.expired[corr] {
			return nil, sessionContext("Kafka correlation ID reused after timeout")
		}
		if _, ok := k.pending[corr]; ok {
			return nil, sessionContext("Kafka correlation ID reused while pending")
		}
		if api == 0 && info["Acks"] == int16(0) {
			info["Response Expected"] = false
			return info, nil
		}
		if len(k.pending) >= f.a.budget.MaxCollectionElements {
			return nil, protocolError(ErrResourceExceeded, "Kafka pending request count")
		}
		if err := f.reserveSession(256 + int64(len(k.pending)+1)*64 + int64(len(k.expired))*24); err != nil {
			return nil, err
		}
		k.pending[corr] = kafkaPending{api, ver, e.ID, e.Timestamp}
		e.TransactionID = e.ID
		info["Outstanding"] = true
		return info, nil
	}
	corr := int32(binary.BigEndian.Uint32(raw[4:8]))
	info["Correlation ID"] = corr
	pending, ok := k.pending[corr]
	if !ok {
		info["Unmatched"] = true
		info["Context Level"] = "missing-request-version"
		return info, nil
	}
	delete(k.pending, corr)
	api := pending.api
	e.ResponseTo = pending.id
	e.TransactionID = pending.id
	info["API Version"] = pending.version
	info["Latency NS"] = e.Timestamp.Sub(pending.ts).Nanoseconds()
	if err := f.reserveSession(256 + int64(len(k.pending))*64 + int64(len(k.expired))*24); err != nil {
		return nil, err
	}
	info["Matched Request"] = true
	info["API Name"] = kafkaSessionAPIName(api)
	info["API Key"] = api
	body := raw[8:]
	parsed, err := stream_parser.KafkaResponsePayloadVersion(api, pending.version, body)
	if err != nil {
		return nil, err
	}
	for key, v := range parsed {
		info[key] = v
	}
	return info, nil
}

func kafkaSessionAPIName(api int16) string {
	switch api {
	case 0:
		return "Produce"
	case 1:
		return "Fetch"
	case 3:
		return "Metadata"
	case 18:
		return "ApiVersions"
	}
	return "Unknown"
}
