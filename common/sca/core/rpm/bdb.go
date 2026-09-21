// BDB hash/overflow layout derived from go-rpmdb v0.1.0 pkg/bdb (MIT).
// Copyright (c) 2019 Teppei Fukuda. See LICENSE.
package rpm

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

func (s *snapshot) bdb(h []byte, emit func([]byte) error) error {
	var order binary.ByteOrder = le
	if le.Uint32(h[12:]) == 0x61150600 {
		order = be
	}
	if order.Uint32(h[12:]) != 0x61561 || h[25] != 8 {
		return fmt.Errorf("unsupported_syntax: RPM database format")
	}
	if h[24] != 0 {
		return fmt.Errorf("unsupported_syntax: encrypted BDB")
	}
	size := int(order.Uint32(h[20:]))
	last := uint64(order.Uint32(h[32:]))
	if size < 512 || size > 65536 || size&(size-1) != 0 || (last+1)*uint64(size) > uint64(s.size) {
		return fmt.Errorf("malformed_input: BDB page dimensions")
	}
	for number := uint64(1); number <= last; number++ {
		page, err := s.at(int64(number)*int64(size), size)
		if err != nil {
			return err
		}
		if page[25] != 2 && page[25] != 13 {
			continue
		}
		entries := int(order.Uint16(page[20:]))
		if entries%2 != 0 || 26+2*entries > size {
			return fmt.Errorf("malformed_input: BDB index dimensions")
		}
		for i := 1; i < entries; i += 2 {
			pos := int(order.Uint16(page[26+i*2:]))
			if pos < 26+2*entries || pos >= size {
				return fmt.Errorf("malformed_input: BDB value offset")
			}
			if page[pos] != 3 {
				continue
			} // Frozen upstream contract: RPM values stored off-page.
			if pos+12 > size {
				return fmt.Errorf("malformed_input: BDB overflow reference")
			}
			next := order.Uint32(page[pos+4:])
			length := int(order.Uint32(page[pos+8:]))
			if length > s.limits.MaxRecordBytes {
				return scanerr.New(scanerr.ResourceLimit, "BDB record")
			}
			raw := make([]byte, 0, length)
			seen := map[uint32]bool{}
			for next != 0 {
				if seen[next] || uint64(next) > last {
					return fmt.Errorf("malformed_input: BDB overflow cycle or page")
				}
				seen[next] = true
				b, err := s.at(int64(next)*int64(size), size)
				if err != nil {
					return err
				}
				if b[25] != 7 {
					return fmt.Errorf("malformed_input: BDB non-overflow page")
				}
				next = order.Uint32(b[16:])
				n := size - 26
				if next == 0 {
					n = int(order.Uint16(b[22:]))
				}
				if n > size-26 || n > length-len(raw) {
					return fmt.Errorf("malformed_input: BDB overflow length")
				}
				raw = append(raw, b[26:26+n]...)
			}
			if len(raw) != length {
				return fmt.Errorf("malformed_input: BDB truncated value")
			}
			if err := emit(raw); err != nil {
				return err
			}
		}
	}
	return nil
}
