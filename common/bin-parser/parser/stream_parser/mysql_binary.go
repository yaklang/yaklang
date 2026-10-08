package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// DecodeMySQLColumnPacket reuses the classic column field reader. Its caller
// supplies the negotiated MariaDB extended-metadata capability explicitly.
func DecodeMySQLColumnPacket(w []byte, maria bool) (map[string]any, error) {
	if len(w) < 4 || len(w) > mysqlFieldsMaxBytes {
		return nil, fmt.Errorf("mysql: column packet size")
	}
	r := &mysqlFieldsReader{wire: w, end: len(w)}
	s, i, _ := r.packet()
	v := r.column(maria)
	r.finishPacket(s, i, "Column")
	if r.at != len(w) {
		r.fail("trailing column bytes")
	}
	return v, r.err
}

// DecodeMySQLBinaryValue decodes one parameter/row scalar and returns exactly
// consumed bytes. NULL comes from the caller's bitmap, never from a value byte.
func DecodeMySQLBinaryValue(w []byte, typ byte, unsigned bool) (any, int, error) {
	n := 0
	switch typ {
	case 1:
		n = 1
	case 2, 13:
		n = 2
	case 3, 9, 4:
		n = 4
	case 8, 5:
		n = 8
	case 6:
		return nil, 0, nil
	}
	if n > 0 {
		if len(w) < n {
			return nil, 0, fmt.Errorf("mysql: truncated numeric")
		}
		var u uint64
		for i, b := range w[:n] {
			u |= uint64(b) << uint(8*i)
		}
		if typ == 4 {
			v := math.Float32frombits(uint32(u))
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return map[string]any{"Float": fmt.Sprint(v), "IEEE754 Bits": u}, n, nil
			}
			return v, n, nil
		}
		if typ == 5 {
			v := math.Float64frombits(u)
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return map[string]any{"Float": fmt.Sprint(v), "IEEE754 Bits": u}, n, nil
			}
			return v, n, nil
		}
		if unsigned {
			return u, n, nil
		}
		shift := uint(64 - n*8)
		return int64(u<<shift) >> shift, n, nil
	}
	switch typ {
	case 7, 10, 11, 12:
		if len(w) == 0 {
			return nil, 0, fmt.Errorf("mysql: missing temporal length")
		}
		n = int(w[0])
		if len(w) < 1+n {
			return nil, 0, fmt.Errorf("mysql: truncated temporal")
		}
		allowed := n == 0 || n == 4 || n == 7 || n == 11
		if typ == 10 {
			allowed = n == 0 || n == 4
		}
		if typ == 11 {
			allowed = n == 0 || n == 8 || n == 12
		}
		if !allowed {
			return nil, 0, fmt.Errorf("mysql: invalid temporal length")
		}
		v := map[string]any{"Zero": n == 0}
		b := w[1 : 1+n]
		if n > 0 {
			if typ == 11 {
				if b[0] > 1 {
					return nil, 0, fmt.Errorf("mysql: invalid temporal sign")
				}
				v["Negative"] = b[0] == 1
				v["Days"] = binary.LittleEndian.Uint32(b[1:])
				v["Hour"] = b[5]
				v["Minute"] = b[6]
				v["Second"] = b[7]
				if n == 12 {
					v["Microseconds"] = binary.LittleEndian.Uint32(b[8:])
				}
			} else {
				v["Year"] = binary.LittleEndian.Uint16(b)
				v["Month"] = b[2]
				v["Day"] = b[3]
				if n >= 7 {
					v["Hour"] = b[4]
					v["Minute"] = b[5]
					v["Second"] = b[6]
				}
				if n == 11 {
					v["Microseconds"] = binary.LittleEndian.Uint32(b[7:])
				}
			}
		}
		return v, 1 + n, nil
	case 0, 15, 16, 245, 246, 247, 248, 249, 250, 251, 252, 253, 254, 255:
		r := &mysqlFieldsReader{wire: w, end: len(w)}
		b, _, _ := r.sized("Binary Value", "raw", false)
		return bytes.Clone(b), r.at, r.err
	default:
		return nil, 0, fmt.Errorf("mysql: unsupported binary type %d", typ)
	}
}
