package pcaputil

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Validate block lengths before pcapgo can allocate from an untrusted pcapng
// header. Section endianness may change. Packet/option blocks are capped at
// 16 MiB and interface tables at 1024 entries per section.
type boundedNgInput struct {
	input      io.Reader
	header     [12]byte
	prefix     []byte
	remaining  int64
	order      binary.ByteOrder
	interfaces int
}

func (r *boundedNgInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.prefix) > 0 {
		n := copy(p, r.prefix)
		r.prefix = r.prefix[n:]
		return n, nil
	}
	if r.remaining > 0 {
		if int64(len(p)) > r.remaining {
			p = p[:r.remaining]
		}
		n, err := r.input.Read(p)
		r.remaining -= int64(n)
		if err == io.EOF && r.remaining > 0 {
			err = io.ErrUnexpectedEOF
		}
		return n, err
	}
	if _, err := io.ReadFull(r.input, r.header[:8]); err != nil {
		return 0, err
	}
	headerLen := 8
	section := binary.LittleEndian.Uint32(r.header[:4]) == 0x0a0d0d0a
	if section {
		if _, err := io.ReadFull(r.input, r.header[8:12]); err != nil {
			return 0, err
		}
		switch binary.LittleEndian.Uint32(r.header[8:12]) {
		case 0x1a2b3c4d:
			r.order = binary.LittleEndian
		case 0x4d3c2b1a:
			r.order = binary.BigEndian
		default:
			return 0, fmt.Errorf("invalid pcapng byte order")
		}
		r.interfaces = 0
		headerLen = 12
	}
	if r.order == nil {
		return 0, fmt.Errorf("pcapng must start with a section header")
	}
	length := r.order.Uint32(r.header[4:8])
	if length < 12 || length > 16<<20 || length%4 != 0 || (section && length < 28) {
		return 0, fmt.Errorf("pcapng block length %d exceeds bounded reader limits", length)
	}
	if r.order.Uint32(r.header[:4]) == 1 {
		r.interfaces++
		if r.interfaces > 1024 {
			return 0, fmt.Errorf("pcapng has too many interfaces in a section")
		}
	}
	r.remaining = int64(length) - int64(headerLen)
	r.prefix = r.header[:headerLen]
	n := copy(p, r.prefix)
	r.prefix = r.prefix[n:]
	return n, nil
}
