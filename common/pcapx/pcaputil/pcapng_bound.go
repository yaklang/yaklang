package pcaputil

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/gopacket/gopacket/pcapgo"
)

const maxCaptureBlock = 16 << 20

// NewBoundedNgReader validates packet headers before pcapgo sees allocation
// lengths. Complete blocks (including options and trailers) are checked before
// publication. Unknown private blocks are bounded; NRB/DSB are explicitly
// unsupported until their independently retained tables have budgets.
func NewBoundedNgReader(input io.Reader, options pcapgo.NgReaderOptions) (*pcapgo.NgReader, error) {
	return pcapgo.NewNgReader(&boundedNgInput{input: input}, options)
}

type boundedNgInput struct {
	input              io.Reader
	prefix             []byte
	block              []byte
	order              binary.ByteOrder
	snaplens           []uint32
	sections, metadata int
	err                error
}

func (r *boundedNgInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.prefix) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		if err := r.nextBlock(); err != nil {
			r.err = err
			return 0, err
		}
	}
	n := copy(p, r.prefix)
	r.prefix = r.prefix[n:]
	return n, nil
}

func (r *boundedNgInput) nextBlock() error {
	var head [28]byte
	if _, err := io.ReadFull(r.input, head[:8]); err != nil {
		return err
	}
	read := 8
	section := binary.LittleEndian.Uint32(head[:4]) == 0x0a0d0d0a
	if section {
		if _, err := io.ReadFull(r.input, head[8:12]); err != nil {
			return shortNg(err)
		}
		read = 12
		switch binary.LittleEndian.Uint32(head[8:12]) {
		case 0x1a2b3c4d:
			r.order = binary.LittleEndian
		case 0x4d3c2b1a:
			r.order = binary.BigEndian
		default:
			return fmt.Errorf("pcapng: invalid byte order")
		}
		r.sections++
		if r.sections > 1024 {
			return fmt.Errorf("pcapng: section budget")
		}
		r.snaplens = r.snaplens[:0]
		r.metadata = 0
	}
	if r.order == nil {
		return fmt.Errorf("pcapng: missing section")
	}
	kind, length := r.order.Uint32(head[:4]), r.order.Uint32(head[4:8])
	if length < 12 || length > maxCaptureBlock || length%4 != 0 {
		return fmt.Errorf("pcapng: block length %d exceeds bounded reader limits or is invalid", length)
	}
	fixed := 8
	switch kind {
	case 0x0a0d0d0a:
		fixed = 24
	case 1:
		fixed = 16
	case 2, 6:
		fixed = 28
	case 3:
		fixed = 12
	case 5:
		fixed = 20
	case 4, 10:
		return fmt.Errorf("pcapng: unsupported retained-metadata block %d", kind)
	}
	if int(length) < fixed+4 {
		return fmt.Errorf("pcapng: block shorter than header")
	}
	if _, err := io.ReadFull(r.input, head[read:fixed]); err != nil {
		return shortNg(err)
	}
	options := fixed
	switch kind {
	case 1:
		snap := r.order.Uint32(head[12:16])
		if snap > maxCaptureBlock {
			return fmt.Errorf("pcapng: snaplen exceeds budget")
		}
		if len(r.snaplens) >= 1024 {
			return fmt.Errorf("pcapng: interface budget")
		}
		r.snaplens = append(r.snaplens, snap)
	case 2, 3, 6:
		iface := uint32(0)
		if kind == 2 {
			iface = uint32(r.order.Uint16(head[8:10]))
		} else if kind == 6 {
			iface = r.order.Uint32(head[8:12])
		}
		if uint64(iface) >= uint64(len(r.snaplens)) {
			return fmt.Errorf("pcapng: unknown interface %d", iface)
		}
		var captured, original uint32
		if kind == 3 {
			original = r.order.Uint32(head[8:12])
			captured = original
			if snap := r.snaplens[iface]; snap != 0 && captured > snap {
				captured = snap
			}
		} else {
			captured = r.order.Uint32(head[20:24])
			original = r.order.Uint32(head[24:28])
		}
		available := length - uint32(fixed) - 4
		if captured > maxCaptureBlock || captured > original || captured > available || (r.snaplens[iface] != 0 && captured > r.snaplens[iface]) {
			return fmt.Errorf("pcapng: invalid capture length %d (original=%d available=%d)", captured, original, available)
		}
		padded := (captured + 3) &^ uint32(3)
		if padded > available {
			return fmt.Errorf("pcapng: truncated packet padding")
		}
		options = fixed + int(padded)
		if kind == 3 && padded != available {
			return fmt.Errorf("pcapng: simple packet length mismatch")
		}
	case 5:
		if uint64(r.order.Uint32(head[8:12])) >= uint64(len(r.snaplens)) {
			return fmt.Errorf("pcapng: statistics interface missing")
		}
	}
	// Allocation is bounded by block length, and packet caplen/snaplen were
	// checked above using only a fixed-size stack header.
	if cap(r.block) < int(length) {
		r.block = make([]byte, length)
	} else {
		r.block = r.block[:length]
	}
	copy(r.block, head[:fixed])
	if _, err := io.ReadFull(r.input, r.block[fixed:]); err != nil {
		return shortNg(err)
	}
	if r.order.Uint32(r.block[len(r.block)-4:]) != length {
		return fmt.Errorf("pcapng: block trailer mismatch")
	}
	switch kind {
	case 0x0a0d0d0a, 1, 2, 5, 6:
		if err := r.validateOptions(kind, r.block[options:len(r.block)-4]); err != nil {
			return err
		}
	}
	if kind == 1 || kind == 5 || section {
		r.metadata += len(r.block)
		if r.metadata > 1<<20 {
			return fmt.Errorf("pcapng: section metadata budget")
		}
	}
	r.prefix = r.block
	return nil
}

func shortNg(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

func (r *boundedNgInput) validateOptions(kind uint32, data []byte) error {
	if len(data) > 64<<10 {
		return fmt.Errorf("pcapng: option byte budget")
	}
	for count := 0; len(data) > 0; count++ {
		if count >= 4096 || len(data) < 4 {
			return fmt.Errorf("pcapng: invalid option header/count")
		}
		code, n := r.order.Uint16(data[:2]), int(r.order.Uint16(data[2:4]))
		data = data[4:]
		padded := (n + 3) &^ 3
		if padded > len(data) {
			return fmt.Errorf("pcapng: truncated option")
		}
		if code == 0 {
			if n != 0 || len(data) != 0 {
				return fmt.Errorf("pcapng: invalid end option")
			}
			return nil
		}
		if kind == 1 {
			if (code == 9 && n != 1) || (code == 14 && n != 8) || (code == 11 && n < 1) {
				return fmt.Errorf("pcapng: invalid interface option length")
			}
			if code == 9 {
				exp := data[0] & 0x7f
				if (data[0]&0x80 != 0 && exp > 63) || (data[0]&0x80 == 0 && exp > 19) {
					return fmt.Errorf("pcapng: timestamp resolution overflow")
				}
			}
		}
		if kind == 5 && code >= 2 && code <= 8 && n != 8 {
			return fmt.Errorf("pcapng: invalid statistics option length")
		}
		data = data[padded:]
	}
	return nil
}
