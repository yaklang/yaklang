package pcaputil

import (
	"crypto/sha256"
	"fmt"
	"net"
	"strconv"
)

const tftpMaximumFileBytes = 16 << 20

// WithProtocolTFTPFileAssembly selects bounded observed octet RRQ/WRQ files on
// one initial UDP service port. Files are returned as owned bytes only after
// the captured final ACK; nothing is written to a filename or filesystem.
// This does not authenticate a server or prove remote storage. Netascii and
// windowed transfers retain their separate unsupported/unknown boundaries.
func WithProtocolTFTPFileAssembly(port uint16, maximum int) CaptureOption {
	return func(c *CaptureConfig) error {
		if port == 0 || maximum < 1 || maximum > tftpMaximumFileBytes {
			return fmt.Errorf("TFTP file assembly requires an initial port and 1..16777216 bytes")
		}
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		if c.datagramDecodeAs[port] != "" {
			return fmt.Errorf("TFTP file service conflicts with explicit DecodeAs")
		}
		c.binParserConfig.TFTPFilePort, c.binParserConfig.TFTPFileBytesLimit = port, maximum
		return nil
	}
}

// WithSessionTFTPFileAssembly selects the same finite UDP observation profile.
func WithSessionTFTPFileAssembly(port uint16, maximum int) ProtocolSessionOption {
	return func(s *captureSession) error {
		if port == 0 || maximum < 1 || maximum > tftpMaximumFileBytes {
			return fmt.Errorf("TFTP file assembly requires an initial port and 1..16777216 bytes")
		}
		if s.f.a.datagramDecodeAs[port] != "" {
			return fmt.Errorf("TFTP file service conflicts with explicit DecodeAs")
		}
		s.f.a.config.TFTPFilePort, s.f.a.config.TFTPFileBytesLimit = port, maximum
		return nil
	}
}

// Evidence names the first accepted DATA event, in the final event's capture
// domain. Packet numbers remain empty for byte-feed input; none are invented.
type tftpFileEvidence struct {
	ID   uint64
	Refs []PacketReference
}

func (e tftpFileEvidence) numbers() []uint64 {
	n := make([]uint64, len(e.Refs))
	for i, r := range e.Refs {
		n[i] = r.Number
	}
	return n
}

type binTFTPFile struct {
	limit                              int
	data                               []byte
	blocked                            bool
	requestID, negotiationID           uint64
	requestPackets, negotiationPackets []uint64
	sources                            []map[string]any
	metadata                           int64
	reserve                            func(int64) error
}

func (s *binTFTPFile) allocated() int64 {
	return int64(cap(s.data)) + s.metadata + int64(cap(s.sources))*8
}
func (s *binTFTPFile) reserveStorage(dataCapacity int, metadata int64, sourceCapacity int) error {
	if s.reserve != nil {
		return s.reserve(int64(dataCapacity) + metadata + int64(sourceCapacity)*8)
	}
	return nil
}
func (s *binTFTPFile) context(e tftpFileEvidence, max int, negotiation bool) error {
	if len(e.Refs) > max || 1+len(s.sources)+tftpFileBoolInt(negotiation) > max {
		return protocolError(ErrResourceExceeded, "TFTP file evidence collection limit")
	}
	cost := int64(128 + 8*len(e.Refs))
	if err := s.reserveStorage(cap(s.data), s.metadata+cost, cap(s.sources)); err != nil {
		return err
	}
	if negotiation {
		s.negotiationID, s.negotiationPackets = e.ID, e.numbers()
	} else {
		s.requestID, s.requestPackets = e.ID, e.numbers()
	}
	s.metadata += cost
	return nil
}
func tftpFileBoolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (a *binParser) newTFTPFile(peer string) *binTFTPFile {
	_, p, err := net.SplitHostPort(peer)
	if err != nil {
		return nil
	}
	port, err := strconv.ParseUint(p, 10, 16)
	if err != nil || uint16(port) != a.config.TFTPFilePort || a.config.TFTPFileBytesLimit <= 0 {
		return nil
	}
	return &binTFTPFile{limit: a.config.TFTPFileBytesLimit}
}

func (s *binTFTPFile) append(w []byte, e tftpFileEvidence, max int) error {
	if s.blocked {
		return protocolError(ErrContextRequired, "TFTP file bytes are no longer contiguous")
	}
	if len(w) > s.limit-len(s.data) {
		s.abort()
		return protocolError(ErrResourceExceeded, "TFTP observed file exceeds byte limit")
	}
	if len(e.Refs) > max || 2+len(s.sources)+tftpFileBoolInt(s.negotiationID != 0) > max {
		return protocolError(ErrResourceExceeded, "TFTP file evidence collection limit")
	}
	n := len(s.data) + len(w)
	capacity := cap(s.data)
	if n > capacity {
		capacity = min(s.limit, tftpFileMaxInt(n, tftpFileMaxInt(512, 2*capacity)))
	}
	sc := cap(s.sources)
	if len(s.sources) == sc {
		sc = tftpFileMaxInt(1, 2*sc)
	}
	meta := s.metadata + int64(512+8*len(e.Refs))
	if err := s.reserveStorage(capacity, meta, sc); err != nil {
		s.abort()
		return err
	}
	if capacity != cap(s.data) {
		grown := make([]byte, len(s.data), capacity)
		copy(grown, s.data)
		s.data = grown
	}
	if sc != cap(s.sources) {
		grown := make([]map[string]any, len(s.sources), sc)
		copy(grown, s.sources)
		s.sources = grown
	}
	s.sources = append(s.sources, map[string]any{"Event ID": e.ID, "File Offset": len(s.data), "Event Offset": 4, "Length": len(w), "Packet Numbers": e.numbers()})
	s.metadata = meta
	s.data = append(s.data, w...)
	return nil
}
func tftpFileMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func (s *binTFTPFile) clear() {
	s.data = nil
	s.sources = nil
	s.requestPackets = nil
	s.negotiationPackets = nil
	s.metadata = 0
	if s.reserve != nil {
		_ = s.reserve(0)
	}
}
func (s *binTFTPFile) abort() { s.blocked = true; s.clear() }
func (s *binTFTPFile) complete(out map[string]any) {
	if s.blocked {
		return
	}
	file := make([]byte, len(s.data))
	copy(file, s.data)
	out["File Bytes"] = file
	out["File Byte Count"] = len(s.data)
	out["File SHA256"] = fmt.Sprintf("%x", sha256.Sum256(s.data))
	out["File Request ID"] = s.requestID
	out["File Request Packets"] = append([]uint64{}, s.requestPackets...)
	if s.negotiationID != 0 {
		out["File Negotiation ID"] = s.negotiationID
		out["File Negotiation Packets"] = append([]uint64{}, s.negotiationPackets...)
	}
	out["File Sources"] = cloneSessionValue(s.sources)
	s.blocked = true
	s.clear()
}
