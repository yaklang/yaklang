package stream_parser

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

var dccpCRC32CTable = crc32.MakeTable(crc32.Castagnoli)

// validateDCCPDataChecksums verifies every well-sized RFC 4340 9.3 option.
// The CRC is computed once even if repeated options cover a large payload.
// All numerical DCCP option values use network byte order (RFC 4340 3.1).
// Nonsensical option lengths terminate the option area, per section 5.8.
func validateDCCPDataChecksums(data []byte) (int, error) {
	if len(data) < 12 || len(data) > 65535 {
		return 0, fmt.Errorf("dccp: datagram size is outside bounds")
	}
	kind, extended := (data[8]>>1)&15, data[8]&1 != 0
	if kind > 9 || (!extended && (kind < 2 || kind > 4)) {
		return 0, fmt.Errorf("dccp: invalid packet type or sequence format")
	}
	pos := 12
	if extended {
		pos = 16
	}
	if kind != 0 && kind != 2 {
		pos += 4
		if extended {
			pos += 4
		}
	}
	if kind == 0 || kind == 1 || kind == 7 {
		pos += 4
	}
	header := int(data[4]) * 4
	if header < pos || header > len(data) {
		return 0, fmt.Errorf("dccp: data offset outside packet header bounds")
	}
	count := 0
	var computed uint32
	for pos < header {
		kind := data[pos]
		if kind < 32 {
			pos++
			continue
		}
		if pos+1 == header {
			break
		}
		length := int(data[pos+1])
		if length < 2 || length > header-pos {
			break
		}
		if kind == 44 && length == 6 {
			if count == 0 {
				computed = crc32.Checksum(data[header:], dccpCRC32CTable)
			}
			if binary.BigEndian.Uint32(data[pos+2:pos+6]) != computed {
				return 0, fmt.Errorf("dccp: application data CRC32c mismatch")
			}
			count++
		}
		pos += length
	}
	return count, nil
}
