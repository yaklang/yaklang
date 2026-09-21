package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// DecodeProtobufWire describes wire fields only. Length-delimited values are
// opaque: without a schema, string/message/packed interpretations are guesses.
func DecodeProtobufWire(b []byte, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("positive protobuf field budget required")
	}
	var rows []map[string]any
	for len(b) > 0 {
		if len(rows) >= limit {
			return nil, protocolError(ErrResourceExceeded, "protobuf field count")
		}
		key, n := binary.Uvarint(b)
		if n <= 0 || key>>3 == 0 || key>>3 > 0x1fffffff {
			return nil, fmt.Errorf("invalid protobuf field key")
		}
		b = b[n:]
		row := map[string]any{"Number": key >> 3, "Wire Type": key & 7}
		size := 0
		switch key & 7 {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				return nil, fmt.Errorf("truncated protobuf varint")
			}
			row["Value"] = v
			size = n
		case 1:
			size = 8
		case 2:
			v, n := binary.Uvarint(b)
			if n <= 0 || v > uint64(len(b)-n) {
				return nil, fmt.Errorf("protobuf length")
			}
			b = b[n:]
			size = int(v)
			row["Length"] = v
		case 5:
			size = 4
		default:
			return nil, protocolError(ErrUnsupportedFeature, "protobuf groups are outside the wire profile")
		}
		if size > len(b) {
			return nil, fmt.Errorf("truncated protobuf fixed field")
		}
		if key&7 == 1 {
			row["Value"] = binary.LittleEndian.Uint64(b)
		}
		if key&7 == 5 {
			row["Value"] = binary.LittleEndian.Uint32(b)
		}
		rows = append(rows, row)
		b = b[size:]
	}
	return rows, nil
}
