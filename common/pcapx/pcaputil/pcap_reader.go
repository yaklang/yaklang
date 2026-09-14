package pcaputil

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// classicPcapReader borrows packet bytes directly from a bounded read buffer.
// The caller must consume each packet before the next read. Large records use
// one reusable, bounded overflow buffer; malformed lengths never drive an
// unbounded allocation. Other formats remain on the native libpcap path.
type classicPcapReader struct {
	input   *bufio.Reader
	order   binary.ByteOrder
	scale   int64
	snaplen uint32
	link    layers.LinkType
	large   []byte
}

func newClassicPcapReader(input io.Reader) (*classicPcapReader, error) {
	r := &classicPcapReader{input: bufio.NewReaderSize(input, 256<<10), scale: 1000}
	header, err := r.input.Peek(24)
	if err != nil {
		return nil, err
	}
	switch binary.LittleEndian.Uint32(header[:4]) {
	case 0xa1b2c3d4:
		r.order = binary.LittleEndian
	case 0xd4c3b2a1:
		r.order = binary.BigEndian
	case 0xa1b23c4d:
		r.order, r.scale = binary.LittleEndian, 1
	case 0x4d3cb2a1:
		r.order, r.scale = binary.BigEndian, 1
	default:
		return nil, fmt.Errorf("not a classic pcap")
	}
	// Extended link flags (for example FCS metadata) retain native handling.
	if r.order.Uint32(header[20:24]) > 0xffff {
		return nil, fmt.Errorf("extended pcap link metadata requires native reader")
	}
	// Reuse the established global-header validation (version and link type).
	validated, err := pcapgo.NewReader(r.input)
	if err != nil {
		return nil, err
	}
	r.snaplen, r.link = validated.Snaplen(), validated.LinkType()
	if r.snaplen == 0 || r.snaplen > 16<<20 {
		return nil, fmt.Errorf("pcap snapshot length requires native reader")
	}
	return r, nil
}

func (r *classicPcapReader) read() ([]byte, gopacket.CaptureInfo, error) {
	var ci gopacket.CaptureInfo
	header, err := r.input.Peek(16)
	if err != nil {
		if err == io.EOF && len(header) != 0 {
			err = io.ErrUnexpectedEOF
		}
		return nil, ci, err
	}
	ci.Timestamp = time.Unix(int64(r.order.Uint32(header[:4])), int64(r.order.Uint32(header[4:8]))*r.scale)
	captured, length := r.order.Uint32(header[8:12]), r.order.Uint32(header[12:16])
	ci.CaptureLength, ci.Length = int(captured), int(length)
	if captured > r.snaplen || captured > length {
		return nil, ci, fmt.Errorf("invalid pcap record length: captured=%d wire=%d snaplen=%d", captured, length, r.snaplen)
	}
	r.input.Discard(16)
	var raw []byte
	if int(captured) <= r.input.Size() {
		raw, err = r.input.Peek(int(captured))
		if err == nil {
			r.input.Discard(int(captured))
		}
	} else {
		if cap(r.large) < int(captured) {
			r.large = make([]byte, captured)
		}
		raw = r.large[:captured]
		_, err = io.ReadFull(r.input, raw)
	}
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return raw, ci, err
}
