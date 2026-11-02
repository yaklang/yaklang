// Package trafficoracle joins reference observations to captured byte positions.
// It does not infer application identity or treat reference stream IDs as wire generations.
package trafficoracle

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strconv"
)

type Field struct {
	Name, Value, Show string
	Position, Size    int
}

type Protocol struct {
	Name           string
	Position, Size int
	Fields         []Field
}

type Packet struct {
	Number, Interface, CapturedLength uint64
	Protocols                         []Protocol
}

// Evidence must come from the same original capture as PDML. IPOffset is an
// independently verified outer IPv4 offset, not an occurrence index in TSV.
// Full captured bytes permit exact header checks even for invalid wire lengths.
type Evidence struct {
	Number        uint64 `json:"frame"`
	Interface     uint64 `json:"interface"`
	Section       uint64 `json:"section"`
	Encapsulation string `json:"encapsulation"`
	IPOffset      int    `json:"ip_offset"`
	Captured      []byte `json:"captured"`
}

type UDPObservation struct {
	Number           uint64 `json:"frame"`
	Interface        uint64 `json:"interface"`
	Section          uint64 `json:"section"`
	Encapsulation    string `json:"encapsulation"`
	Stream           uint64 `json:"reference_stream"`
	IPOffset         int    `json:"ip_offset"`
	UDPOffset        int    `json:"udp_offset"`
	Source           string `json:"source"`
	Destination      string `json:"destination"`
	SourcePort       uint16 `json:"source_port"`
	DestinationPort  uint16 `json:"destination_port"`
	Length           uint16 `json:"udp_length"`
	ExtentConsistent bool   `json:"extent_consistent"`
	CapturedSHA256   string `json:"captured_sha256"`
}

func attr(start xml.StartElement, name string) string {
	for _, a := range start.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func position(start xml.StartElement, name string) (int, error) {
	n, err := strconv.ParseUint(attr(start, name), 10, 32)
	if err != nil || n > 1<<24 {
		return 0, fmt.Errorf("invalid PDML %s", name)
	}
	return int(n), nil
}

func relevant(name string) bool {
	switch name {
	case "frame.number", "frame.interface_id", "frame.cap_len", "ip.src", "ip.dst",
		"udp.srcport", "udp.dstport", "udp.length", "udp.checksum", "udp.stream":
		return true
	}
	return false
}

func readProtocol(d *xml.Decoder, start xml.StartElement) (Protocol, error) {
	p := Protocol{Name: attr(start, "name")}
	var err error
	if p.Position, err = position(start, "pos"); err != nil {
		return p, err
	}
	if p.Size, err = position(start, "size"); err != nil {
		return p, err
	}
	depth, fields := 1, 0
	for depth > 0 {
		token, err := d.Token()
		if err != nil {
			return p, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			fields++
			if depth > 32 || fields > 10000 || t.Name.Local != "field" {
				return p, fmt.Errorf("unsupported PDML field tree")
			}
			name := attr(t, "name")
			if relevant(name) {
				f := Field{Name: name, Value: attr(t, "value"), Show: attr(t, "show")}
				if len(f.Value) > 128 || len(f.Show) > 128 {
					return p, fmt.Errorf("oversized PDML header field")
				}
				if f.Position, err = position(t, "pos"); err != nil {
					return p, err
				}
				if f.Size, err = position(t, "size"); err != nil {
					return p, err
				}
				p.Fields = append(p.Fields, f)
			}
		case xml.EndElement:
			depth--
		}
	}
	return p, nil
}

func one(fields []Field, name string) (Field, error) {
	var found Field
	count := 0
	for _, f := range fields {
		if f.Name == name {
			found, count = f, count+1
		}
	}
	if count != 1 {
		return found, fmt.Errorf("need one %s, got %d", name, count)
	}
	return found, nil
}

func metadata(p Protocol, name string) (uint64, error) {
	f, err := one(p.Fields, name)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(f.Show, 10, 64)
}

func readPacket(d *xml.Decoder) (Packet, error) {
	var p Packet
	frames := 0
	for {
		token, err := d.Token()
		if err != nil {
			return p, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			name := attr(t, "name")
			if t.Name.Local != "proto" || name != "frame" && name != "ip" && name != "udp" {
				if err := d.Skip(); err != nil {
					return p, err
				}
				continue
			}
			proto, err := readProtocol(d, t)
			if err != nil {
				return p, err
			}
			if name == "frame" {
				frames++
				if p.Number, err = metadata(proto, "frame.number"); err != nil {
					return p, err
				}
				if p.Interface, err = metadata(proto, "frame.interface_id"); err != nil {
					return p, err
				}
				if p.CapturedLength, err = metadata(proto, "frame.cap_len"); err != nil {
					return p, err
				}
			} else {
				p.Protocols = append(p.Protocols, proto)
				if len(p.Protocols) > 128 {
					return p, fmt.Errorf("too many reference network headers")
				}
			}
		case xml.EndElement:
			if t.Name.Local == "packet" {
				if frames != 1 || p.Number == 0 {
					return p, fmt.Errorf("missing or ambiguous reference frame metadata")
				}
				return p, nil
			}
		}
	}
}

// ReadPDML streams each reference packet. Duplicate or decreasing frame numbers,
// missing interface/length metadata and truncated XML are errors, not skipped tests.
func ReadPDML(r io.Reader, visit func(Packet) error) error {
	d := xml.NewDecoder(r)
	var previous uint64
	for {
		token, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if t, ok := token.(xml.StartElement); ok && t.Name.Local == "packet" {
			p, err := readPacket(d)
			if err != nil {
				return err
			}
			if p.Number <= previous {
				return fmt.Errorf("duplicate or unordered reference frame %d", p.Number)
			}
			previous = p.Number
			if err := visit(p); err != nil {
				return err
			}
		}
	}
}

func wireField(p Protocol, name string, at int, raw []byte) error {
	f, err := one(p.Fields, name)
	if err != nil {
		return err
	}
	decoded, err := hex.DecodeString(f.Value)
	if err != nil || f.Position != at || f.Size != len(raw) || !bytes.Equal(decoded, raw) {
		return fmt.Errorf("%s does not match captured position/size/bytes", name)
	}
	return nil
}

// MatchDirectIPv4UDP selects by exact IP/UDP positions and original header bytes.
// It deliberately supports only direct unfragmented IPv4→UDP. Nested UDP remains
// in Packet.Protocols and cannot be flattened into this parent's stream set.
// ExtentConsistent is a length observation, not checksum or application validation.
func MatchDirectIPv4UDP(p Packet, e Evidence) (UDPObservation, error) {
	var out UDPObservation
	b, at := e.Captured, e.IPOffset
	if p.Number != e.Number || p.Interface != e.Interface || p.CapturedLength != uint64(len(b)) || len(b) > 1<<20 || at < 0 || at > len(b)-20 {
		return out, fmt.Errorf("capture frame/domain/length/offset mismatch")
	}
	h := int(b[at]&15) * 4
	if b[at]>>4 != 4 || h < 20 || h > len(b)-at-8 || b[at+9] != 17 || binary.BigEndian.Uint16(b[at+6:at+8])&0x3fff != 0 {
		return out, fmt.Errorf("unsupported or incomplete direct IPv4 UDP header")
	}
	u := at + h
	ipMatches, udpMatches := 0, 0
	var selected Protocol
	for _, proto := range p.Protocols {
		if proto.Name == "ip" && proto.Position == at && proto.Size == h {
			if err := wireField(proto, "ip.src", at+12, b[at+12:at+16]); err != nil {
				return out, err
			}
			if err := wireField(proto, "ip.dst", at+16, b[at+16:at+20]); err != nil {
				return out, err
			}
			ipMatches++
		}
		if proto.Name == "udp" && proto.Position == u && proto.Size == 8 {
			for i, name := range []string{"udp.srcport", "udp.dstport", "udp.length", "udp.checksum"} {
				if err := wireField(proto, name, u+i*2, b[u+i*2:u+i*2+2]); err != nil {
					return out, err
				}
			}
			selected, udpMatches = proto, udpMatches+1
		}
	}
	if ipMatches != 1 || udpMatches != 1 {
		return out, fmt.Errorf("need one source-matched IP/UDP header, got %d/%d", ipMatches, udpMatches)
	}
	stream, err := metadata(selected, "udp.stream")
	if err != nil {
		return out, err
	}
	length, ipLength := binary.BigEndian.Uint16(b[u+4:u+6]), int(binary.BigEndian.Uint16(b[at+2:at+4]))
	hash := sha256.Sum256(b)
	out = UDPObservation{Number: e.Number, Interface: e.Interface, Section: e.Section, Encapsulation: e.Encapsulation, Stream: stream, IPOffset: at, UDPOffset: u,
		Source: net.IP(b[at+12 : at+16]).String(), Destination: net.IP(b[at+16 : at+20]).String(),
		SourcePort: binary.BigEndian.Uint16(b[u : u+2]), DestinationPort: binary.BigEndian.Uint16(b[u+2 : u+4]), Length: length,
		ExtentConsistent: ipLength >= h+8 && ipLength <= len(b)-at && length >= 8 && int(length) <= ipLength-h,
		CapturedSHA256:   hex.EncodeToString(hash[:])}
	return out, nil
}
