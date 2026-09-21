package stream_parser

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const kafkaMaxBytes = 1 << 20

var kafkaAPIVersions = map[int16][2]int16{
	0:  {0, 7},  // Produce, non-flexible
	1:  {0, 11}, // Fetch, non-flexible
	3:  {0, 8},  // Metadata, non-flexible
	18: {0, 2},  // ApiVersions, non-flexible
}

func parseKafkaFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeKafkaFields(w, profile)
	}, "kafka-fields", "big")
}

func decodeKafkaFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) < 8 || len(wire) > kafkaMaxBytes {
		return nil, nil, fmt.Errorf("kafka-fields: 8..1048576 bytes required")
	}
	r := &kafkaReader{wire: wire}
	size := r.i32("Length")
	if r.err != nil || int(size) != len(wire)-4 {
		return nil, nil, fmt.Errorf("kafka-fields: length mismatch")
	}
	info := map[string]any{"Profile": profile, "Context Level": "observed"}
	switch profile {
	case "request":
		r.request(info)
	case "response":
		r.response(info)
	default:
		return nil, nil, fmt.Errorf("kafka-fields: unknown profile %s", profile)
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	if r.at != len(wire) {
		return nil, nil, fmt.Errorf("kafka-fields: trailing bytes")
	}
	return r.fields, info, nil
}

type kafkaReader struct {
	wire   []byte
	at     int
	fields []tlsCertificateField
	err    error
}

func (r *kafkaReader) take(name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.at+n > len(r.wire) {
		r.err = fmt.Errorf("kafka-fields: truncated %s", name)
		return nil
	}
	s := r.at
	r.at += n
	r.fields = append(r.fields, tlsCertificateLeaf(name, typ, s, r.at))
	return r.wire[s:r.at]
}

func (r *kafkaReader) i8(name string) int8 {
	b := r.take(name, "int8", 1)
	if b == nil {
		return 0
	}
	return int8(b[0])
}

func (r *kafkaReader) i16(name string) int16 {
	b := r.take(name, "int16", 2)
	if b == nil {
		return 0
	}
	return int16(binary.BigEndian.Uint16(b))
}

func (r *kafkaReader) i32(name string) int32 {
	b := r.take(name, "int32", 4)
	if b == nil {
		return 0
	}
	return int32(binary.BigEndian.Uint32(b))
}

func (r *kafkaReader) i64(name string) int64 {
	b := r.take(name, "int64", 8)
	if b == nil {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b))
}

func (r *kafkaReader) str(name string) string {
	n := int(r.i16(name + " Length"))
	if n < 0 {
		return ""
	}
	return string(r.take(name, "string", n))
}

func (r *kafkaReader) bytes(name string) []byte {
	n := int(r.i32(name + " Length"))
	if n < 0 {
		return nil
	}
	return r.take(name, "raw", n)
}

func (r *kafkaReader) request(info map[string]any) {
	api := r.i16("API Key")
	ver := r.i16("API Version")
	corr := r.i32("Correlation ID")
	info["API Key"] = api
	info["API Version"] = ver
	info["Correlation ID"] = corr
	info["API Name"] = kafkaAPIName(api)
	info["Client ID"] = r.str("Client ID")
	if r.err != nil {
		return
	}
	rng, ok := kafkaAPIVersions[api]
	if !ok {
		r.err = fmt.Errorf("kafka-fields: API key %d is not in the supported matrix", api)
		return
	}
	if ver < rng[0] || ver > rng[1] {
		r.err = fmt.Errorf("kafka-fields: API %d version %d is outside %d..%d", api, ver, rng[0], rng[1])
		return
	}
	switch api {
	case 18:
		// ApiVersions v0-v2 request body is empty (v1-v2 still empty before flexible).
	case 3:
		r.metadataRequest(ver, info)
	case 0:
		r.produceRequest(ver, info)
	case 1:
		r.fetchRequest(ver, info)
	}
}

func (r *kafkaReader) response(info map[string]any) {
	corr := r.i32("Correlation ID")
	info["Correlation ID"] = corr
	if r.at < len(r.wire) {
		r.take("Body", "raw", len(r.wire)-r.at)
	}
}

func (r *kafkaReader) metadataRequest(ver int16, info map[string]any) {
	n := int(r.i32("Topics Count"))
	if n < -1 || (n == -1 && ver == 0) {
		r.err = fmt.Errorf("kafka-fields: invalid nullable topics")
		return
	}
	info["All Topics"] = n == -1 || (n == 0 && ver == 0)
	var names []string
	for i := 0; i < n && r.err == nil; i++ {
		names = append(names, r.str("Topic Name"))
	}
	info["Topics"] = names
	if ver >= 4 {
		info["Allow Auto Topic Creation"] = r.boolean("Allow Auto Topic Creation")
	}
	if ver >= 8 {
		info["Include Cluster Authorized Operations"] = r.boolean("Include Cluster Authorized Operations")
		info["Include Topic Authorized Operations"] = r.boolean("Include Topic Authorized Operations")
	}
}

func (r *kafkaReader) boolean(name string) bool {
	b := r.i8(name)
	if b != 0 && b != 1 {
		r.err = fmt.Errorf("kafka-fields: invalid boolean %s", name)
	}
	return b == 1
}

func (r *kafkaReader) produceRequest(ver int16, info map[string]any) {
	if ver >= 3 {
		info["Transactional ID"] = r.str("Transactional ID")
	}
	info["Acks"] = r.i16("Acks")
	info["Timeout"] = r.i32("Timeout")
	n := int(r.i32("Topics Count"))
	for i := 0; i < n && r.err == nil; i++ {
		topic := r.str("Topic Name")
		info["Topic Name"] = topic
		pc := int(r.i32("Partition Count"))
		for p := 0; p < pc && r.err == nil; p++ {
			info["Partition"] = r.i32("Partition")
			n := int(r.i32("Record Set Length"))
			if n == 0 {
				continue
			}
			if n < 0 {
				r.err = fmt.Errorf("kafka-fields: null Record Set is not supported")
				return
			}
			end := r.at + n
			if end > len(r.wire) {
				r.err = fmt.Errorf("kafka-fields: truncated Record Set")
				return
			}
			r.recordBatch(info)
			if r.err == nil && r.at != end {
				r.err = fmt.Errorf("kafka-fields: Record Set boundary mismatch")
			}
		}
	}
}

func (r *kafkaReader) fetchRequest(ver int16, info map[string]any) {
	info["Replica ID"] = r.i32("Replica ID")
	info["Max Wait Time"] = r.i32("Max Wait Time")
	info["Min Bytes"] = r.i32("Min Bytes")
	if ver >= 3 {
		info["Max Bytes"] = r.i32("Max Bytes")
	}
	if ver >= 4 {
		info["Isolation Level"] = r.i8("Isolation Level")
	}
	if ver >= 7 {
		info["Session ID"] = r.i32("Session ID")
		info["Session Epoch"] = r.i32("Session Epoch")
	}
	n := int(r.i32("Topics Count"))
	for i := 0; i < n && r.err == nil; i++ {
		info["Topic Name"] = r.str("Topic Name")
		pc := int(r.i32("Partition Count"))
		for p := 0; p < pc && r.err == nil; p++ {
			info["Partition"] = r.i32("Partition")
			if ver >= 9 {
				info["Current Leader Epoch"] = r.i32("Current Leader Epoch")
			}
			info["Fetch Offset"] = r.i64("Fetch Offset")
			if ver >= 5 {
				info["Log Start Offset"] = r.i64("Log Start Offset")
			}
			info["Partition Max Bytes"] = r.i32("Partition Max Bytes")
		}
	}
	if ver >= 7 {
		n := r.i32("Forgotten Topics Count")
		if n < 0 {
			r.err = fmt.Errorf("kafka-fields: negative forgotten topics")
		}
		for i := int32(0); i < n && r.err == nil; i++ {
			r.str("Forgotten Topic Name")
			pc := r.i32("Forgotten Partition Count")
			if pc < 0 {
				r.err = fmt.Errorf("kafka-fields: negative forgotten partitions")
			}
			for j := int32(0); j < pc && r.err == nil; j++ {
				r.i32("Forgotten Partition")
			}
		}
	}
	if ver >= 11 {
		info["Rack ID"] = r.str("Rack ID")
	}

}

func (r *kafkaReader) recordBatch(info map[string]any) {
	info["Base Offset"] = r.i64("Base Offset")
	batchLen := int(r.i32("Batch Length"))
	start := r.at
	if batchLen < 49 || start+batchLen > len(r.wire) {
		r.err = fmt.Errorf("kafka-fields: invalid RecordBatch length")
		return
	}
	r.i32("Partition Leader Epoch")
	magic := r.i8("Magic")
	r.take("CRC", "raw", 4)
	attr := r.i16("Attributes")
	comp := attr & 7
	info["Magic"] = magic
	info["Compression"] = comp
	if magic != 2 {
		r.err = fmt.Errorf("kafka-fields: RecordBatch magic %d is not 2", magic)
		return
	}
	if comp != 0 {
		r.err = fmt.Errorf("kafka-fields: compressed RecordBatch is outside the supported matrix")
		return
	}
	r.i32("Last Offset Delta")
	r.i64("Base Timestamp")
	r.i64("Max Timestamp")
	r.i64("Producer ID")
	r.i16("Producer Epoch")
	r.i32("Base Sequence")
	count := r.i32("Records Count")
	info["Records Count"] = count
	remain := start + batchLen - r.at
	if remain < 0 {
		r.err = fmt.Errorf("kafka-fields: RecordBatch boundary mismatch")
		return
	}
	r.take("Records", "raw", remain)
}

func kafkaAPIName(api int16) string {
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

func ParseKafkaResponseBody(api int16, body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return map[string]any{}, nil
	}
	r := &kafkaReader{wire: body}
	info := map[string]any{"API Name": kafkaAPIName(api)}
	switch api {
	case 18:
		info["Error Code"] = r.i16("Error Code")
		n := int(r.i32("API Keys Count"))
		var keys []int16
		for i := 0; i < n && r.err == nil; i++ {
			k := r.i16("API Key")
			r.i16("Min Version")
			r.i16("Max Version")
			keys = append(keys, k)
		}
		info["API Keys"] = keys
	case 3:
		bc := int(r.i32("Broker Count"))
		for i := 0; i < bc && r.err == nil; i++ {
			r.i32("Node ID")
			r.str("Host")
			r.i32("Port")
		}
		tc := int(r.i32("Topic Count"))
		var topics []string
		for i := 0; i < tc && r.err == nil; i++ {
			r.i16("Topic Error")
			topics = append(topics, r.str("Topic Name"))
			pc := int(r.i32("Partition Count"))
			for p := 0; p < pc && r.err == nil; p++ {
				r.i16("Partition Error")
				r.i32("Partition")
				r.i32("Leader")
				rc := int(r.i32("Replica Count"))
				for j := 0; j < rc && r.err == nil; j++ {
					r.i32("Replica")
				}
				ic := int(r.i32("ISR Count"))
				for j := 0; j < ic && r.err == nil; j++ {
					r.i32("ISR")
				}
			}
		}
		info["Topics"] = topics
	case 0:
		n := int(r.i32("Topic Count"))
		for i := 0; i < n && r.err == nil; i++ {
			info["Topic Name"] = r.str("Topic Name")
			pc := int(r.i32("Partition Count"))
			for p := 0; p < pc && r.err == nil; p++ {
				info["Partition"] = r.i32("Partition")
				info["Error Code"] = r.i16("Error Code")
				info["Base Offset"] = r.i64("Base Offset")
			}
		}
	case 1:
		n := int(r.i32("Topic Count"))
		for i := 0; i < n && r.err == nil; i++ {
			info["Topic Name"] = r.str("Topic Name")
			pc := int(r.i32("Partition Count"))
			for p := 0; p < pc && r.err == nil; p++ {
				info["Partition"] = r.i32("Partition")
				info["Error Code"] = r.i16("Error Code")
				info["High Watermark"] = r.i64("High Watermark")
				set := r.bytes("Record Set")
				if len(set) > 0 && r.err == nil {
					br := &kafkaReader{wire: set}
					br.recordBatch(info)
					if br.err != nil {
						return nil, br.err
					}
				}
			}
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	return info, nil
}
