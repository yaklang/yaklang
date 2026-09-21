// NDB slot/blob traversal derives from go-rpmdb v0.1.0 pkg/ndb/ndb.go.
// Copyright (c) 2021 SUSE LLC. MIT license, see LICENSE-SUSE.
package rpm

import (
	"fmt"
	"hash/adler32"
)

func (s *snapshot) ndb(h []byte, emit func([]byte) error) error {
	if le.Uint32(h[4:]) != 0 {
		return fmt.Errorf("unsupported_syntax: NDB version")
	}
	pages := uint64(le.Uint32(h[12:]))
	if pages == 0 || pages > 2048 || pages*4096 > uint64(s.size) {
		return fmt.Errorf("malformed_input: NDB slot pages")
	}
	slots, err := s.at(32, int(pages*4096)-32)
	if err != nil {
		return err
	}
	seen := map[uint32]bool{}
	for i := 0; i < len(slots); i += 16 {
		slot := slots[i : i+16]
		if string(slot[:4]) != "Slot" {
			return fmt.Errorf("malformed_input: NDB slot magic")
		}
		id := le.Uint32(slot[4:])
		if id == 0 {
			continue
		}
		if seen[id] {
			return fmt.Errorf("malformed_input: NDB duplicate package")
		}
		seen[id] = true
		offset, n := uint64(le.Uint32(slot[8:]))*16, uint64(le.Uint32(slot[12:]))*16
		if n < 32 || n > uint64(s.limits.MaxRecordBytes)+32 || offset < pages*4096 {
			return fmt.Errorf("malformed_input: NDB block dimensions")
		}
		b, err := s.at(int64(offset), int(n))
		if err != nil {
			return err
		}
		if string(b[:4]) != "BlbS" || le.Uint32(b[4:]) != id {
			return fmt.Errorf("malformed_input: NDB blob identity")
		}
		length := uint64(le.Uint32(b[12:]))
		if length+28 > n || (length+28+15)/16 != n/16 {
			return fmt.Errorf("malformed_input: NDB blob length")
		}
		tail := b[n-12:]
		if string(tail[8:]) != "BlbE" || uint64(le.Uint32(tail[4:])) != length || le.Uint32(tail) != adler32.Checksum(b[:n-12]) {
			return fmt.Errorf("malformed_input: NDB checksum or tail")
		}
		if err := emit(b[16 : 16+length]); err != nil {
			return err
		}
	}
	return nil
}
