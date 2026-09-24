package stream_parser

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

type kafkaRecordBudget struct{ bytes, count int }

var kafkaCRC = crc32.MakeTable(crc32.Castagnoli)

// ParseKafkaRecordSet validates every batch and exposes bounded records. Only
// none/gzip codecs are supported; CRC covers the original compressed bytes.
func ParseKafkaRecordSet(w []byte) ([]map[string]any, error) {
	if len(w) > kafkaMaxBytes {
		return nil, fmt.Errorf("kafka: record set exceeds limit")
	}
	left := kafkaMaxBytes
	count := 4096
	return kafkaRecords(w, 0, &left, &count)
}
func kafkaInflate(w []byte, left *int) ([]byte, error) {
	z, err := gzip.NewReader(bytes.NewReader(w))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	b, err := io.ReadAll(io.LimitReader(z, int64(*left)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > *left {
		return nil, fmt.Errorf("kafka: inflated record budget exceeded")
	}
	*left -= len(b)
	return b, nil
}
func kafkaRecords(w []byte, depth int, left, count *int) ([]map[string]any, error) {
	if depth > 4 {
		return nil, fmt.Errorf("kafka: compressed message nesting")
	}
	var out []map[string]any
	for len(w) > 0 {
		if len(out) >= 4096 || len(w) < 17 {
			return nil, fmt.Errorf("kafka: truncated record header")
		}
		n := int(binary.BigEndian.Uint32(w[8:12]))
		if n < 5 || n > len(w)-12 {
			return nil, fmt.Errorf("kafka: record length")
		}
		body := w[12 : 12+n]
		magic := body[4]
		info := map[string]any{"Base Offset": int64(binary.BigEndian.Uint64(w[:8])), "Magic": int8(magic)}
		if magic == 2 {
			if n < 49 {
				return nil, fmt.Errorf("kafka: short batch")
			}
			if crc32.Checksum(body[9:], kafkaCRC) != binary.BigEndian.Uint32(body[5:9]) {
				return nil, fmt.Errorf("kafka: batch CRC mismatch")
			}
			attr := int16(binary.BigEndian.Uint16(body[9:11]))
			compression := attr & 7
			info["Compression"] = compression
			info["CRC Valid"] = true
			info["Last Offset Delta"] = int32(binary.BigEndian.Uint32(body[11:15]))
			info["Base Timestamp"] = int64(binary.BigEndian.Uint64(body[15:23]))
			info["Max Timestamp"] = int64(binary.BigEndian.Uint64(body[23:31]))
			info["Producer ID"] = int64(binary.BigEndian.Uint64(body[31:39]))
			info["Producer Epoch"] = int16(binary.BigEndian.Uint16(body[39:41]))
			info["Base Sequence"] = int32(binary.BigEndian.Uint32(body[41:45]))
			c := int32(binary.BigEndian.Uint32(body[45:49]))
			if c < 0 || int(c) > *count {
				return nil, fmt.Errorf("kafka: record count limit")
			}
			*count -= int(c)
			data := body[49:]
			if compression == 1 {
				var err error
				data, err = kafkaInflate(data, left)
				if err != nil {
					return nil, err
				}
			} else if compression != 0 {
				return nil, fmt.Errorf("%w: compression %d", ErrKafkaUnsupported, compression)
			}
			rd := kafkaRecordReader{w: data}
			var records []map[string]any
			for i := int32(0); i < c && rd.err == nil; i++ {
				length := rd.vint(32)
				raw := rd.take(length)
				if rd.err != nil {
					break
				}
				r := kafkaRecordReader{w: raw}
				attribute := r.take(1)
				timestamp := r.vint(64)
				offset := r.vint(32)
				key := r.blob(true)
				value := r.blob(true)
				hc := r.vint(32)
				if hc < 0 || hc > int64(*count) {
					return nil, fmt.Errorf("kafka: record headers limit")
				}
				*count -= int(hc)
				var headers []map[string]any
				for j := int64(0); j < hc && r.err == nil; j++ {
					headers = append(headers, map[string]any{"Key": r.blob(false), "Value": r.blob(true)})
				}
				if r.err != nil {
					return nil, r.err
				}
				if len(r.w) != 0 || offset < 0 || offset > int64(info["Last Offset Delta"].(int32)) {
					return nil, fmt.Errorf("kafka: record boundary or offset")
				}
				records = append(records, map[string]any{"Attributes": attribute[0], "Timestamp Delta": timestamp, "Offset Delta": offset, "Key": key, "Value": value, "Headers": headers})
			}
			if rd.err != nil {
				return nil, rd.err
			}
			if len(rd.w) != 0 {
				return nil, fmt.Errorf("kafka: records count or trailing bytes")
			}
			info["Records Count"] = c
			info["Records"] = records
		} else if magic == 0 || magic == 1 {
			if crc32.ChecksumIEEE(body[4:]) != binary.BigEndian.Uint32(body[:4]) {
				return nil, fmt.Errorf("kafka: message CRC mismatch")
			}
			r := &kafkaReader{wire: body, at: 5}
			attr := r.i8("Attributes")
			if magic == 1 {
				info["Timestamp"] = r.i64("Timestamp")
			}
			key := r.bytes("Key")
			value := r.bytes("Value")
			if r.err != nil {
				return nil, r.err
			}
			if r.at != len(body) {
				return nil, fmt.Errorf("kafka: message trailing bytes")
			}
			comp := attr & 7
			info["Compression"] = int16(comp)
			info["CRC Valid"] = true
			if comp == 1 {
				data, err := kafkaInflate(value, left)
				if err != nil {
					return nil, err
				}
				nested, err := kafkaRecords(data, depth+1, left, count)
				if err != nil {
					return nil, err
				}
				info["Batches"] = nested
			} else if comp == 0 {
				if *count == 0 {
					return nil, fmt.Errorf("kafka: message count limit")
				}
				*count--
				info["Key"] = bytes.Clone(key)
				info["Value"] = bytes.Clone(value)
				info["Records Count"] = int32(1)
			} else {
				return nil, fmt.Errorf("%w: compression %d", ErrKafkaUnsupported, comp)
			}
		} else {
			return nil, fmt.Errorf("%w: magic %d", ErrKafkaUnsupported, magic)
		}
		out = append(out, info)
		w = w[12+n:]
	}
	return out, nil
}

type kafkaRecordReader struct {
	w   []byte
	err error
}

func (r *kafkaRecordReader) take(n int64) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > int64(len(r.w)) {
		r.err = fmt.Errorf("kafka: truncated record field")
		return nil
	}
	v := r.w[:n]
	r.w = r.w[n:]
	return bytes.Clone(v)
}
func (r *kafkaRecordReader) vint(bits int) int64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Varint(r.w)
	if n <= 0 || bits == 32 && (v < -2147483648 || v > 2147483647) {
		r.err = fmt.Errorf("kafka: invalid record varint")
		return 0
	}
	r.w = r.w[n:]
	return v
}
func (r *kafkaRecordReader) blob(nullable bool) []byte {
	n := r.vint(32)
	if n == -1 && nullable {
		return nil
	}
	return r.take(n)
}
