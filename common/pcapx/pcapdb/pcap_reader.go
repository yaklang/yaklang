package pcapdb

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

const maxCaptureBlock = 16 << 20

type packetLocation struct {
	section                             int64
	iface                               int
	recordOffset, dataOffset, endOffset int64
	hasTimestamp                        bool
}

type captureInterface struct {
	section    int64
	id         int
	link       layers.LinkType
	snaplen    uint32
	resolution byte
	offset     int64
	name       string
}

type packetReader struct {
	read          func() ([]byte, gopacket.CaptureInfo, error)
	ng            *pcapgo.NgReader
	positions     *ngLocationInput
	link          layers.LinkType
	classicOffset int64
	interfaces    []captureInterface
	format        string
}

func newPacketReader(file *os.File) (*packetReader, error) {
	var magic [4]byte
	if _, err := file.ReadAt(magic[:], 0); err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	r := &packetReader{classicOffset: 24, format: "pcap"}
	if binary.LittleEndian.Uint32(magic[:]) == 0x0a0d0d0a {
		r.format = "pcapng"
		r.positions = &ngLocationInput{input: file}
		ng, err := pcaputil.NewBoundedNgReader(r.positions, pcapgo.NgReaderOptions{WantMixedLinkType: true})
		if err != nil {
			return nil, err
		}
		r.ng, r.read = ng, ng.ZeroCopyReadPacketData
	} else {
		classic, err := pcaputil.NewBoundedPcapReader(file)
		if err != nil {
			return nil, err
		}
		r.read, r.link = classic.ReadPacketData, classic.LinkType()
		resolution := byte(6)
		if value := binary.LittleEndian.Uint32(magic[:]); value == 0xa1b23c4d || value == 0x4d3cb2a1 {
			resolution = 9
		}
		r.interfaces = []captureInterface{{link: r.link, snaplen: classic.Snaplen(), resolution: resolution}}
	}
	return r, nil
}

func (r *packetReader) next() ([]byte, gopacket.CaptureInfo, packetLocation, error) {
	raw, ci, err := r.read()
	if err != nil {
		return nil, ci, packetLocation{}, err
	}
	if r.ng == nil {
		loc := packetLocation{recordOffset: r.classicOffset, dataOffset: r.classicOffset + 16, endOffset: r.classicOffset + 16 + int64(len(raw)), hasTimestamp: true}
		r.classicOffset = loc.endOffset
		return raw, ci, loc, nil
	}
	if len(r.positions.packets) == 0 {
		return nil, ci, packetLocation{}, fmt.Errorf("pcapdb: pcapng packet location missing")
	}
	loc := r.positions.packets[0]
	r.positions.packets = r.positions.packets[1:]
	if loc.iface != ci.InterfaceIndex {
		return nil, ci, loc, fmt.Errorf("pcapdb: pcapng interface mismatch")
	}
	iface, err := r.ng.Interface(ci.InterfaceIndex)
	if err != nil {
		return nil, ci, loc, err
	}
	r.link = iface.LinkType
	return raw, ci, loc, nil
}

func (r *packetReader) takeInterfaces() []captureInterface {
	if r.positions != nil {
		interfaces := r.positions.interfaces
		r.positions.interfaces = nil
		return interfaces
	}
	interfaces := r.interfaces
	r.interfaces = nil
	return interfaces
}

// This adapter observes complete bounded blocks before the existing hardened
// reader validates them. It records physical offsets, independent of bufio read
// ahead and ProtocolEvent's logical stream offsets. It does not decode packets.
type ngLocationInput struct {
	input           io.Reader
	buffer, pending []byte
	order           binary.ByteOrder
	offset          int64
	section         int64
	seenSection     bool
	interfaceCount  int
	packets         []packetLocation
	interfaces      []captureInterface
}

func (r *ngLocationInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		if err := r.nextBlock(); err != nil {
			return 0, err
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *ngLocationInput) nextBlock() error {
	var header [12]byte
	if _, err := io.ReadFull(r.input, header[:8]); err != nil {
		return err
	}
	section := binary.LittleEndian.Uint32(header[:4]) == 0x0a0d0d0a
	headLen := 8
	if section {
		if _, err := io.ReadFull(r.input, header[8:]); err != nil {
			return err
		}
		headLen = 12
		switch binary.LittleEndian.Uint32(header[8:]) {
		case 0x1a2b3c4d:
			r.order = binary.LittleEndian
		case 0x4d3c2b1a:
			r.order = binary.BigEndian
		default:
			return fmt.Errorf("pcapdb: invalid pcapng byte order")
		}
		if r.seenSection {
			r.section++
		}
		r.seenSection, r.interfaceCount = true, 0
	}
	if r.order == nil {
		return fmt.Errorf("pcapdb: pcapng section header missing")
	}
	kind, length := r.order.Uint32(header[:4]), r.order.Uint32(header[4:8])
	if length < uint32(headLen+4) || length > maxCaptureBlock || length%4 != 0 {
		return fmt.Errorf("pcapdb: invalid pcapng block length %d", length)
	}
	if cap(r.buffer) < int(length) {
		r.buffer = make([]byte, length)
	} else {
		r.buffer = r.buffer[:length]
	}
	copy(r.buffer, header[:headLen])
	if _, err := io.ReadFull(r.input, r.buffer[headLen:]); err != nil {
		return err
	}
	if r.order.Uint32(r.buffer[length-4:]) != length {
		return fmt.Errorf("pcapdb: pcapng block trailer mismatch")
	}
	loc := packetLocation{section: r.section, recordOffset: r.offset, endOffset: r.offset + int64(length), hasTimestamp: true}
	switch kind {
	case 1:
		if length < 20 {
			return fmt.Errorf("pcapdb: truncated interface description")
		}
		iface := captureInterface{section: r.section, id: r.interfaceCount, link: layers.LinkType(r.order.Uint16(r.buffer[8:10])), snaplen: r.order.Uint32(r.buffer[12:16]), resolution: 6}
		for options := r.buffer[16 : len(r.buffer)-4]; len(options) >= 4; {
			code, n := r.order.Uint16(options[:2]), int(r.order.Uint16(options[2:4]))
			options = options[4:]
			if code == 0 {
				break
			}
			padded := (n + 3) &^ 3
			if padded > len(options) {
				return fmt.Errorf("pcapdb: truncated interface option")
			}
			switch code {
			case 2:
				iface.name = string(options[:n])
			case 9:
				if n == 1 {
					iface.resolution = options[0]
				}
			case 14:
				if n == 8 {
					iface.offset = int64(r.order.Uint64(options[:8]))
				}
			}
			options = options[padded:]
		}
		r.interfaceCount++
		r.interfaces = append(r.interfaces, iface)
	case 2, 6:
		if length < 32 {
			return fmt.Errorf("pcapdb: truncated packet block")
		}
		if kind == 2 {
			loc.iface = int(r.order.Uint16(r.buffer[8:10]))
		} else {
			loc.iface = int(r.order.Uint32(r.buffer[8:12]))
		}
		loc.dataOffset = r.offset + 28
		r.packets = append(r.packets, loc)
	case 3:
		if length < 16 {
			return fmt.Errorf("pcapdb: truncated simple packet block")
		}
		loc.dataOffset, loc.hasTimestamp = r.offset+12, false
		r.packets = append(r.packets, loc)
	}
	r.offset += int64(length)
	r.pending = r.buffer
	return nil
}
