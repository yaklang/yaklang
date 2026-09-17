package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type binKafka struct {
	client  int
	pending map[int32]int16
}

func kafkaSupported(api, ver uint16) bool {
	rng, ok := map[uint16][2]uint16{0: {0, 7}, 1: {0, 11}, 3: {0, 8}, 18: {0, 2}}[api]
	return ok && ver >= rng[0] && ver <= rng[1]
}

func probeKafka(w []byte, limit int) ProbeResult {
	if len(w) < 8 {
		if len(w) > 0 && w[0] == 0 {
			return probeNeed("kafka", "request", len(w), 8)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	size := int(binary.BigEndian.Uint32(w[:4]))
	if size < 8 || size > 1<<20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	api := binary.BigEndian.Uint16(w[4:6])
	ver := binary.BigEndian.Uint16(w[6:8])
	if !kafkaSupported(api, ver) {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("kafka", kafkaSessionAPIName(int16(api)), 88)
}

func (f *binFlow) frameKafka(dir int, w []byte) (int, *binSpec, error) {
	k := f.kafka
	if k == nil {
		return 0, nil, sessionContext("Kafka session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(k.pending))*16); err != nil {
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

func (k *binKafka) consume(dir int, raw []byte, result map[string]any) (map[string]any, error) {
	if len(raw) < 8 {
		return nil, fmt.Errorf("kafka: truncated message")
	}
	info := map[string]any{"Context Level": "observed"}
	if meta, ok := result["metadata"].(map[string]any); ok {
		for _, key := range []string{"API Name", "API Key", "API Version", "Correlation ID", "Client ID", "Topic Name", "Magic", "Compression", "Records Count", "Base Offset", "Partition"} {
			if v, ok := meta[key]; ok {
				info[key] = v
			}
		}
	}
	request := k.client < 0 || dir == k.client
	if request {
		if k.client < 0 {
			k.client = dir
		}
		api := int16(binary.BigEndian.Uint16(raw[4:6]))
		corr := int32(binary.BigEndian.Uint32(raw[8:12]))
		info["API Key"] = api
		info["Correlation ID"] = corr
		info["API Name"] = kafkaSessionAPIName(api)
		k.pending[corr] = api
		info["Outstanding"] = true
		return info, nil
	}
	corr := int32(binary.BigEndian.Uint32(raw[4:8]))
	info["Correlation ID"] = corr
	api, ok := k.pending[corr]
	if !ok {
		info["Unmatched"] = true
		return info, nil
	}
	delete(k.pending, corr)
	info["Matched Request"] = true
	info["API Name"] = kafkaSessionAPIName(api)
	info["API Key"] = api
	body := raw[8:]
	parsed, err := stream_parser.ParseKafkaResponseBody(api, body)
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
