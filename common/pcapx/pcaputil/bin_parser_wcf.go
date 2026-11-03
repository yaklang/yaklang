package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// MC-NMF v1.0 record framing used by WCF net.tcp.
//
// This profile deliberately stops at record and envelope boundaries. It does
// not decode .NET Binary XML, SOAP headers/bodies or application objects.
const (
	wcfRecordVersion         byte = 0x00
	wcfRecordMode            byte = 0x01
	wcfRecordVia             byte = 0x02
	wcfRecordKnownEncoding   byte = 0x03
	wcfRecordExtensible      byte = 0x04
	wcfRecordUnsizedEnvelope byte = 0x05
	wcfRecordSizedEnvelope   byte = 0x06
	wcfRecordEnd             byte = 0x07
	wcfRecordFault           byte = 0x08
	wcfRecordUpgradeRequest  byte = 0x09
	wcfRecordUpgradeResponse byte = 0x0A
	wcfRecordPreambleAck     byte = 0x0B
	wcfRecordPreambleEnd     byte = 0x0C
	wcfMaxEncodedSizeBytes        = 5
	wcfFaultNamespace             = "http://schemas.microsoft.com/ws/2006/05/framing/faults/"
)

type wcfChunk struct {
	size        uint64
	offset      int
	headerLen   int
	headerStart int
}

type wcfRecord struct {
	typ         byte
	total       int
	declared    uint64
	headerStart int
	headerLen   int
	bodyOffset  int
	body        []byte
	chunks      []wcfChunk
	text        string
	major       byte
	minor       byte
	mode        byte
	encoding    byte
}

type binWCF struct {
	clientDir      int
	clientKnown    bool
	midstream      bool
	versionKnown   bool
	major          byte
	minor          byte
	mode           byte
	via            string
	encodingKnown  bool
	encoding       byte
	extensible     string
	upgradeRequest string
	preambleEnded  bool
	preambleAck    bool
	ended          [2]bool
	faulted        [2]bool
	envelopeSeq    uint64
}

func wcfRecordName(typ byte) string {
	switch typ {
	case wcfRecordVersion:
		return "Version"
	case wcfRecordMode:
		return "Mode"
	case wcfRecordVia:
		return "Via"
	case wcfRecordKnownEncoding:
		return "Known Encoding"
	case wcfRecordExtensible:
		return "Extensible Encoding"
	case wcfRecordUnsizedEnvelope:
		return "Unsized Envelope"
	case wcfRecordSizedEnvelope:
		return "Sized Envelope"
	case wcfRecordEnd:
		return "End"
	case wcfRecordFault:
		return "Fault"
	case wcfRecordUpgradeRequest:
		return "Upgrade Request"
	case wcfRecordUpgradeResponse:
		return "Upgrade Response"
	case wcfRecordPreambleAck:
		return "Preamble Ack"
	case wcfRecordPreambleEnd:
		return "Preamble End"
	default:
		return "Reserved"
	}
}

func wcfModeName(mode byte) string {
	switch mode {
	case 0x01:
		return "Singleton-Unsized"
	case 0x02:
		return "Duplex"
	case 0x03:
		return "Simplex"
	case 0x04:
		return "Singleton-Sized"
	default:
		return "Reserved"
	}
}

func wcfEncodingName(encoding byte) string {
	switch encoding {
	case 0x00:
		return "SOAP 1.1 UTF-8"
	case 0x01:
		return "SOAP 1.1 UTF-16"
	case 0x02:
		return "SOAP 1.1 Unicode little-endian"
	case 0x03:
		return "SOAP 1.2 UTF-8"
	case 0x04:
		return "SOAP 1.2 UTF-16"
	case 0x05:
		return "SOAP 1.2 Unicode little-endian"
	case 0x06:
		return "MTOM"
	case 0x07:
		return "Binary"
	case 0x08:
		return "Binary with in-band dictionary"
	default:
		return "Reserved"
	}
}

func wcfReadSize(w []byte, offset, max int) (uint64, int, bool, error) {
	if offset < 0 || offset > max || offset > len(w) {
		return 0, 0, false, nil
	}
	if offset == len(w) {
		return 0, 0, false, nil
	}
	var size uint64
	for i := 0; i < wcfMaxEncodedSizeBytes; i++ {
		at := offset + i
		if at >= max {
			return 0, 0, false, protocolError(ErrResourceExceeded, "wcf: encoded size exceeds record limit")
		}
		if at >= len(w) {
			return 0, 0, false, nil
		}
		b := w[at]
		if i == wcfMaxEncodedSizeBytes-1 && b > 0x07 {
			return 0, 0, false, protocolError(ErrMalformedMessage, "wcf: encoded size exceeds uint32")
		}
		size |= uint64(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			if b == 0 {
				return 0, 0, false, protocolError(ErrMalformedMessage, "wcf: zero encoded size")
			}
			return size, i + 1, true, nil
		}
	}
	return 0, 0, false, protocolError(ErrMalformedMessage, "wcf: unterminated encoded size")
}

func wcfRequirePayload(w []byte, offset int, size uint64, max int, what string) (int, error) {
	if size == 0 {
		return 0, protocolError(ErrMalformedMessage, "wcf: %s has zero length", what)
	}
	if size > uint64(max-offset) {
		return 0, protocolError(ErrResourceExceeded, "wcf: %s exceeds record limit", what)
	}
	total := offset + int(size)
	return total, nil
}

func wcfValidUTF8(v []byte) bool {
	return len(v) > 0 && utf8.Valid(v)
}

func wcfValidMIME(v string) bool {
	if v == "" || !utf8.ValidString(v) {
		return false
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	main := v
	if at := strings.IndexByte(main, ';'); at >= 0 {
		main = main[:at]
	}
	slash := strings.IndexByte(main, '/')
	return slash > 0 && slash < len(main)-1 && strings.IndexByte(main[slash+1:], '/') < 0
}

func parseWCFRecord(w []byte, max int) (wcfRecord, bool, error) {
	var rec wcfRecord
	if max <= 0 {
		return rec, false, protocolError(ErrResourceExceeded, "wcf: invalid record limit")
	}
	if len(w) == 0 {
		return rec, false, nil
	}
	if len(w) > max {
		w = w[:max]
	}
	rec.typ = w[0]
	switch rec.typ {
	case wcfRecordVersion:
		if len(w) < 3 {
			return rec, false, nil
		}
		rec.major, rec.minor, rec.total = w[1], w[2], 3
		return rec, true, nil
	case wcfRecordMode:
		if len(w) < 2 {
			return rec, false, nil
		}
		rec.mode, rec.total = w[1], 2
		return rec, true, nil
	case wcfRecordKnownEncoding:
		if len(w) < 2 {
			return rec, false, nil
		}
		rec.encoding, rec.total = w[1], 2
		return rec, true, nil
	case wcfRecordEnd, wcfRecordUpgradeResponse, wcfRecordPreambleAck, wcfRecordPreambleEnd:
		rec.total = 1
		return rec, true, nil
	case wcfRecordVia, wcfRecordExtensible, wcfRecordFault, wcfRecordUpgradeRequest:
		size, header, complete, err := wcfReadSize(w, 1, max)
		if err != nil {
			return rec, false, err
		}
		if !complete {
			return rec, false, nil
		}
		total, err := wcfRequirePayload(w, 1+header, size, max, wcfRecordName(rec.typ))
		if err != nil {
			return rec, false, err
		}
		rec.declared, rec.headerStart, rec.headerLen = size, 1, header
		rec.total = total
		if len(w) < total {
			return rec, false, nil
		}
		payload := w[1+header : total]
		if !wcfValidUTF8(payload) {
			return rec, false, protocolError(ErrMalformedMessage, "wcf: %s is not UTF-8", wcfRecordName(rec.typ))
		}
		rec.bodyOffset, rec.body, rec.text = 1+header, payload, string(payload)
		if rec.typ == wcfRecordVia && !strings.HasPrefix(strings.ToLower(rec.text), "net.tcp://") {
			return rec, false, protocolError(ErrUnsupportedFeature, "wcf: Via is not a net.tcp URI")
		}
		if rec.typ == wcfRecordExtensible && !wcfValidMIME(rec.text) {
			return rec, false, protocolError(ErrMalformedMessage, "wcf: malformed extensible encoding")
		}
		return rec, true, nil
	case wcfRecordSizedEnvelope:
		size, header, complete, err := wcfReadSize(w, 1, max)
		if err != nil {
			return rec, false, err
		}
		if !complete {
			return rec, false, nil
		}
		total, err := wcfRequirePayload(w, 1+header, size, max, "sized envelope")
		if err != nil {
			return rec, false, err
		}
		rec.declared, rec.headerStart, rec.headerLen = size, 1, header
		rec.total = total
		if len(w) < total {
			return rec, false, nil
		}
		rec.bodyOffset, rec.body = 1+header, w[1+header:total]
		return rec, true, nil
	case wcfRecordUnsizedEnvelope:
		rec.body = w
		at := 1
		for {
			if at >= max {
				return rec, false, protocolError(ErrResourceExceeded, "wcf: unsized envelope exceeds record limit")
			}
			if at >= len(w) {
				return rec, false, nil
			}
			if w[at] == 0 {
				if len(rec.chunks) == 0 {
					return rec, false, protocolError(ErrMalformedMessage, "wcf: unsized envelope has no data chunk")
				}
				rec.total = at + 1
				return rec, true, nil
			}
			size, header, complete, err := wcfReadSize(w, at, max)
			if err != nil {
				return rec, false, err
			}
			if !complete {
				return rec, false, nil
			}
			total, err := wcfRequirePayload(w, at+header, size, max, "envelope chunk")
			if err != nil {
				return rec, false, err
			}
			if len(w) < total {
				return rec, false, nil
			}
			rec.chunks = append(rec.chunks, wcfChunk{size: size, offset: at + header, headerLen: header, headerStart: at})
			at = total
		}
	default:
		return rec, false, protocolError(ErrMalformedMessage, "wcf: reserved record type 0x%02x", rec.typ)
	}
}

func wcfRecordDeclaredBody(rec wcfRecord) []byte {
	if rec.typ != wcfRecordUnsizedEnvelope {
		return rec.body
	}
	total := 0
	for _, chunk := range rec.chunks {
		total += len(chunkBody(rec, chunk))
	}
	out := make([]byte, 0, total)
	for _, chunk := range rec.chunks {
		out = append(out, chunkBody(rec, chunk)...)
	}
	return out
}

func chunkBody(rec wcfRecord, chunk wcfChunk) []byte {
	// Unsized chunks are retained as offsets because the parser only has the
	// original wire slice while framing. Reconstruct through the same slice
	// when the record is consumed.
	if rec.body != nil {
		start := chunk.offset
		return rec.body[start : start+int(chunk.size)]
	}
	return nil
}

func wcfEnvelopeDigest(body []byte) string {
	sum := sha256.Sum256(body)
	var encoded [64]byte
	hex.Encode(encoded[:], sum[:])
	return string(encoded[:])
}

func newBinWCF(clientDir int, clientKnown bool, midstream bool) *binWCF {
	return &binWCF{clientDir: clientDir, clientKnown: clientKnown, midstream: midstream}
}

func (s *binWCF) storage() int64 {
	return 512 + int64(len(s.via)+len(s.extensible)+len(s.upgradeRequest))
}

func (s *binWCF) contextFields() map[string]any {
	fields := map[string]any{
		"Context Level": "preamble",
	}
	if s.midstream {
		fields["Context Level"] = "midstream"
	}
	if s.versionKnown {
		fields["Version"] = fmt.Sprintf("%d.%d", s.major, s.minor)
	}
	if s.mode != 0 {
		fields["Mode"] = s.mode
		fields["Mode Name"] = wcfModeName(s.mode)
	}
	if s.via != "" {
		fields["Via"] = s.via
	}
	if s.encodingKnown {
		fields["Encoding"] = s.encoding
		fields["Encoding Name"] = wcfEncodingName(s.encoding)
	} else if s.extensible != "" {
		fields["Encoding Name"] = s.extensible
	}
	if s.upgradeRequest != "" {
		fields["Upgrade Protocol"] = s.upgradeRequest
	}
	fields["Preamble End Observed"] = s.preambleEnded
	fields["Preamble Ack Observed"] = s.preambleAck
	fields["Envelope Count"] = s.envelopeSeq
	fields["Envelope Decoded"] = false
	fields["Envelope Body Opaque"] = true
	if s.clientKnown {
		fields["Initiator Direction"] = s.clientDir
	}
	return fields
}

func (s *binWCF) role(dir int) string {
	if !s.clientKnown {
		return "unobserved"
	}
	if dir == s.clientDir {
		return "initiator"
	}
	return "receiver"
}

func (s *binWCF) preflight(dir int, rec wcfRecord) error {
	if s.midstream {
		switch rec.typ {
		case wcfRecordVersion, wcfRecordMode, wcfRecordVia, wcfRecordKnownEncoding, wcfRecordExtensible, wcfRecordUpgradeRequest, wcfRecordPreambleEnd:
			if s.envelopeSeq != 0 {
				return protocolError(ErrMalformedMessage, "wcf: preamble record after midstream envelope")
			}
		}
	}
	switch rec.typ {
	case wcfRecordVersion:
		if rec.major != 1 || rec.minor != 0 {
			return protocolError(ErrUnsupportedVersion, "wcf: unsupported framing version %d.%d", rec.major, rec.minor)
		}
		if s.versionKnown || s.envelopeSeq != 0 {
			return protocolError(ErrMalformedMessage, "wcf: repeated Version record")
		}
	case wcfRecordMode:
		if !s.versionKnown || s.mode != 0 || s.envelopeSeq != 0 {
			return protocolError(ErrMalformedMessage, "wcf: Mode record is out of sequence")
		}
		if rec.mode < 1 || rec.mode > 4 {
			return protocolError(ErrUnsupportedVersion, "wcf: unsupported mode 0x%02x", rec.mode)
		}
	case wcfRecordVia:
		if !s.versionKnown || s.mode == 0 || s.via != "" || s.envelopeSeq != 0 {
			return protocolError(ErrMalformedMessage, "wcf: Via record is out of sequence")
		}
	case wcfRecordKnownEncoding, wcfRecordExtensible:
		if !s.versionKnown || s.mode == 0 || s.via == "" || s.encodingKnown || s.extensible != "" || s.envelopeSeq != 0 {
			return protocolError(ErrMalformedMessage, "wcf: encoding record is out of sequence")
		}
		if rec.typ == wcfRecordKnownEncoding && rec.encoding > 0x08 {
			return protocolError(ErrUnsupportedVersion, "wcf: unsupported encoding 0x%02x", rec.encoding)
		}
	case wcfRecordUpgradeRequest:
		if !s.versionKnown || !s.encodingKnown && s.extensible == "" || s.preambleEnded {
			return protocolError(ErrMalformedMessage, "wcf: Upgrade Request is out of sequence")
		}
	case wcfRecordPreambleEnd:
		if s.midstream {
			if s.preambleEnded {
				return protocolError(ErrMalformedMessage, "wcf: Preamble End is repeated")
			}
			break
		}
		if !s.versionKnown || (!s.encodingKnown && s.extensible == "") || s.preambleEnded {
			return protocolError(ErrMalformedMessage, "wcf: Preamble End is out of sequence")
		}
		if s.clientKnown && dir != s.clientDir {
			return protocolError(ErrMalformedMessage, "wcf: Preamble End arrived from the receiver")
		}
	case wcfRecordPreambleAck:
		if s.midstream && !s.preambleEnded {
			break
		}
		if !s.preambleEnded {
			return protocolError(ErrMalformedMessage, "wcf: Preamble Ack arrived before Preamble End")
		}
		if s.clientKnown && dir == s.clientDir {
			return protocolError(ErrMalformedMessage, "wcf: Preamble Ack arrived from the initiator")
		}
	case wcfRecordUnsizedEnvelope:
		if !s.preambleEnded && !s.midstream {
			return protocolError(ErrMalformedMessage, "wcf: Envelope arrived before Preamble End")
		}
		if s.ended[dir] || s.faulted[dir] {
			return protocolError(ErrMalformedMessage, "wcf: Envelope arrived after terminal record")
		}
		if s.mode == 0x04 {
			return protocolError(ErrUnsupportedFeature, "wcf: Singleton-Sized mode has no bounded record framing")
		}
		if s.mode == 0x02 || s.mode == 0x03 {
			return protocolError(ErrMalformedMessage, "wcf: unsized Envelope is not valid in mode %s", wcfModeName(s.mode))
		}
		if s.mode == 0x01 && s.envelopeSeq != 0 {
			return protocolError(ErrMalformedMessage, "wcf: Singleton-Unsized mode has multiple envelopes")
		}
	case wcfRecordSizedEnvelope:
		if !s.preambleEnded && !s.midstream {
			return protocolError(ErrMalformedMessage, "wcf: Envelope arrived before Preamble End")
		}
		if s.ended[dir] || s.faulted[dir] {
			return protocolError(ErrMalformedMessage, "wcf: Envelope arrived after terminal record")
		}
		if s.mode == 0x04 {
			return protocolError(ErrUnsupportedFeature, "wcf: Singleton-Sized mode has no bounded record framing")
		}
		if s.mode == 0x01 {
			return protocolError(ErrMalformedMessage, "wcf: sized Envelope is not valid in mode %s", wcfModeName(s.mode))
		}
	case wcfRecordEnd:
		if !s.preambleEnded && !s.midstream {
			return protocolError(ErrMalformedMessage, "wcf: End record arrived before Preamble End")
		}
		if s.ended[dir] || s.faulted[dir] {
			return protocolError(ErrMalformedMessage, "wcf: End record repeated on one direction")
		}
	case wcfRecordFault:
		if s.faulted[dir] || s.ended[dir] {
			return protocolError(ErrMalformedMessage, "wcf: Fault record arrived after terminal record")
		}
	case wcfRecordUpgradeResponse:
		if s.upgradeRequest == "" || s.faulted[dir] || s.ended[dir] {
			return protocolError(ErrMalformedMessage, "wcf: Upgrade Response has no observed request")
		}
	}
	return nil
}

func (s *binWCF) apply(dir int, rec wcfRecord) (map[string]any, error) {
	if err := s.preflight(dir, rec); err != nil {
		return nil, err
	}
	fields := s.contextFields()
	fields["Role"] = s.role(dir)
	fields["Record Type"] = uint64(rec.typ)
	fields["Record Name"] = wcfRecordName(rec.typ)
	fields["Record Length"] = uint64(rec.total)
	fields["Wire Field Ranges"] = map[string]any{
		"Record Type": []any{uint64(0), uint64(1)},
	}
	ranges := fields["Wire Field Ranges"].(map[string]any)
	switch rec.typ {
	case wcfRecordVersion:
		s.versionKnown, s.major, s.minor = true, rec.major, rec.minor
		fields["Version"] = fmt.Sprintf("%d.%d", rec.major, rec.minor)
		fields["Major Version"] = uint64(rec.major)
		fields["Minor Version"] = uint64(rec.minor)
		ranges["Major Version"] = []any{uint64(1), uint64(2)}
		ranges["Minor Version"] = []any{uint64(2), uint64(3)}
	case wcfRecordMode:
		s.mode = rec.mode
		fields["Mode"] = uint64(rec.mode)
		fields["Mode Name"] = wcfModeName(rec.mode)
		ranges["Mode"] = []any{uint64(1), uint64(2)}
	case wcfRecordVia:
		s.via = rec.text
		fields["Via"] = rec.text
		ranges["Via Length"] = []any{uint64(rec.headerStart), uint64(rec.headerStart + rec.headerLen)}
		ranges["Via"] = []any{uint64(rec.bodyOffset), uint64(rec.total)}
	case wcfRecordKnownEncoding:
		s.encodingKnown, s.encoding = true, rec.encoding
		fields["Encoding"] = uint64(rec.encoding)
		fields["Encoding Name"] = wcfEncodingName(rec.encoding)
		ranges["Encoding"] = []any{uint64(1), uint64(2)}
	case wcfRecordExtensible:
		s.extensible = rec.text
		fields["Encoding Name"] = rec.text
		ranges["Encoding Length"] = []any{uint64(rec.headerStart), uint64(rec.headerStart + rec.headerLen)}
		ranges["Encoding Type"] = []any{uint64(rec.bodyOffset), uint64(rec.total)}
	case wcfRecordUpgradeRequest:
		s.upgradeRequest = rec.text
		fields["Upgrade Protocol"] = rec.text
		fields["Upgrade Protocol Known"] = rec.text == "application/ssl-tls" || rec.text == "application/negotiate"
		ranges["Upgrade Length"] = []any{uint64(rec.headerStart), uint64(rec.headerStart + rec.headerLen)}
		ranges["Upgrade Protocol"] = []any{uint64(rec.bodyOffset), uint64(rec.total)}
	case wcfRecordPreambleEnd:
		s.preambleEnded = true
		fields["Preamble End Observed"] = true
	case wcfRecordPreambleAck:
		s.preambleAck = true
		fields["Preamble Ack Observed"] = true
	case wcfRecordEnd:
		s.ended[dir] = true
		fields["Terminal"] = true
	case wcfRecordFault:
		s.faulted[dir] = true
		fields["Fault"] = rec.text
		fields["Terminal"] = true
		ranges["Fault Length"] = []any{uint64(rec.headerStart), uint64(rec.headerStart + rec.headerLen)}
		ranges["Fault"] = []any{uint64(rec.bodyOffset), uint64(rec.total)}
	case wcfRecordUpgradeResponse:
		fields["Upgrade Accepted"] = true
	case wcfRecordSizedEnvelope:
		s.envelopeSeq++
		fields["Envelope Kind"] = "sized"
		fields["Envelope Sequence"] = s.envelopeSeq
		fields["Declared Length"] = rec.declared
		fields["Payload Length"] = uint64(len(rec.body))
		fields["Envelope Body SHA256"] = wcfEnvelopeDigest(rec.body)
		fields["Envelope Body Opaque"] = true
		ranges["Envelope Length"] = []any{uint64(rec.headerStart), uint64(rec.headerStart + rec.headerLen)}
		ranges["Envelope Body"] = []any{uint64(rec.bodyOffset), uint64(rec.total)}
	case wcfRecordUnsizedEnvelope:
		body := wcfRecordDeclaredBody(rec)
		s.envelopeSeq++
		fields["Envelope Kind"] = "unsized"
		fields["Envelope Sequence"] = s.envelopeSeq
		fields["Payload Length"] = uint64(len(body))
		fields["Envelope Body SHA256"] = wcfEnvelopeDigest(body)
		fields["Envelope Body Opaque"] = true
		fields["Chunk Count"] = uint64(len(rec.chunks))
		chunks := make([]map[string]any, 0, len(rec.chunks))
		for _, chunk := range rec.chunks {
			chunks = append(chunks, map[string]any{
				"Declared Length": chunk.size,
				"Body Offset":     uint64(chunk.offset),
				"Body Length":     chunk.size,
			})
		}
		fields["Chunks"] = chunks
		if len(rec.chunks) > 0 {
			last := rec.chunks[len(rec.chunks)-1]
			ranges["Envelope Body"] = []any{uint64(rec.chunks[0].offset), uint64(last.offset + int(last.size))}
		}
	}
	fields["Context Level"] = "midstream"
	if s.preambleEnded {
		fields["Context Level"] = "preamble-complete"
	}
	return fields, nil
}

func (f *binFlow) frameWCF(dir int, w []byte) (int, *binSpec, error) {
	if f.wcf == nil {
		return 0, nil, sessionContext("WCF session was not observed")
	}
	if f.wcf.mode == 0x04 && (f.wcf.encodingKnown || f.wcf.extensible != "") {
		return 0, nil, protocolError(ErrUnsupportedFeature, "wcf: Singleton-Sized mode has no bounded record framing")
	}
	limit := min(f.a.budget.MaxFrameBytes, f.a.config.MaxMessageBytes)
	rec, complete, err := parseWCFRecord(w, limit)
	if err != nil {
		return 0, nil, err
	}
	if !complete {
		return 0, nil, nil
	}
	if err := f.wcf.preflight(dir, rec); err != nil {
		return 0, nil, err
	}
	return rec.total, &binSpec{entry: "WCF" + strings.ReplaceAll(wcfRecordName(rec.typ), " ", "")}, nil
}

func (f *binFlow) consumeWCF(dir int, e *ProtocolEvent) error {
	if f.wcf == nil {
		return sessionContext("WCF session was not observed")
	}
	rec, complete, err := parseWCFRecord(e.Raw, min(f.a.budget.MaxFrameBytes, f.a.config.MaxMessageBytes))
	if err != nil {
		return err
	}
	if !complete || rec.total != len(e.Raw) {
		return protocolError(ErrMalformedMessage, "wcf: event does not contain exactly one complete record")
	}
	projection := int64(len(rec.text)) + int64(len(rec.chunks))*64 + 256
	if err := f.reserveSession(f.wcf.storage() + projection); err != nil {
		return err
	}
	fields, err := f.wcf.apply(dir, rec)
	if err != nil {
		return err
	}
	e.Session = fields
	e.semanticFields = cloneSession(fields)
	e.Profile, e.Completeness = "wcf-mc-nmf", "record"
	if rec.typ == wcfRecordUpgradeResponse {
		// The next bytes belong to the negotiated carrier. Let normal detection
		// decide whether TLS, Negotiate or an unsupported protocol follows.
		e.Session["Protocol Transition"] = "wcf->carried"
		f.protocol, f.wcf = "", nil
	}
	return nil
}

func wcfProbeNeed(have, want int) ProbeResult {
	if have >= want {
		return ProbeResult{Verdict: ProbeReject}
	}
	return ProbeResult{Verdict: ProbeNeedMore, Protocol: "wcf", NeedBytes: want - have, Confidence: 55}
}

func wcfProbePreamble(w []byte, limit int) ProbeResult {
	if len(w) < 3 {
		if bytes.HasPrefix([]byte{wcfRecordVersion, 0x01, 0x00}, w) {
			return wcfProbeNeed(len(w), 3)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] != wcfRecordVersion {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[1] != 1 || w[2] != 0 {
		return ProbeResult{Verdict: ProbeReject, Reason: "unsupported MC-NMF version"}
	}
	rec, complete, err := parseWCFRecord(w, limit)
	if err != nil || !complete || rec.typ != wcfRecordVersion {
		if err != nil {
			return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}
		}
		return wcfProbeNeed(len(w), 3)
	}
	at := rec.total
	if len(w) < at+2 {
		return wcfProbeNeed(len(w), at+2)
	}
	rec, complete, err = parseWCFRecord(w[at:], limit-at)
	if err != nil || !complete || rec.typ != wcfRecordMode {
		if err != nil {
			return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}
		}
		return wcfProbeNeed(len(w), at+2)
	}
	if rec.mode < 1 || rec.mode > 4 {
		return ProbeResult{Verdict: ProbeReject, Reason: "unsupported MC-NMF mode"}
	}
	at += rec.total
	if len(w) <= at {
		return wcfProbeNeed(len(w), at+1)
	}
	if w[at] != wcfRecordVia {
		return ProbeResult{Verdict: ProbeReject, Reason: "MC-NMF Via record missing"}
	}
	rec, complete, err = parseWCFRecord(w[at:], limit-at)
	if err != nil {
		if strings.Contains(err.Error(), "not a net.tcp URI") {
			return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}
		}
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}
	}
	if !complete {
		return wcfProbeNeed(len(w), at+rec.total)
	}
	return probeAccept("wcf", "preamble", 100)
}

func wcfTextEnvelopePrefix(payload []byte) (complete bool, possible bool) {
	if len(payload) == 0 {
		return false, false
	}
	text := payload
	if len(text) >= 3 && bytes.Equal(text[:3], []byte{0xef, 0xbb, 0xbf}) {
		text = text[3:]
	} else if len(text) >= 2 && (bytes.Equal(text[:2], []byte{0xff, 0xfe}) || bytes.Equal(text[:2], []byte{0xfe, 0xff})) {
		little := text[0] == 0xff
		text = text[2:]
		units := make([]uint16, 0, len(text)/2)
		for i := 0; i+1 < len(text); i += 2 {
			var unit uint16
			if little {
				unit = uint16(text[i]) | uint16(text[i+1])<<8
			} else {
				unit = uint16(text[i])<<8 | uint16(text[i+1])
			}
			units = append(units, unit)
		}
		text = []byte(string(utf16.Decode(units)))
	}
	if len(text) > 2048 {
		text = text[:2048]
	}
	trimmed := bytes.TrimLeft(text, " \t\r\n")
	if len(trimmed) == 0 {
		return false, true
	}
	if trimmed[0] != '<' {
		return false, false
	}
	end := bytes.IndexByte(trimmed, '>')
	if end < 0 {
		return false, len(trimmed) <= 512
	}
	root := trimmed[1:end]
	if slash := bytes.IndexByte(root, ' '); slash >= 0 {
		root = root[:slash]
	}
	root = bytes.TrimSuffix(root, []byte("/"))
	lower := strings.ToLower(string(root))
	if at := strings.LastIndexByte(lower, ':'); at >= 0 {
		lower = lower[at+1:]
	}
	if lower == "envelope" {
		return true, true
	}
	return false, false
}

func wcfBinaryXMLEnvelopePrefix(payload []byte) (complete bool, possible bool) {
	if len(payload) == 0 {
		return false, false
	}
	typ := payload[0]
	at := 1
	readString := func() (string, bool, bool) {
		size, header, ok, err := wcfReadSize(payload, at, len(payload))
		if err != nil {
			return "", false, false
		}
		if !ok {
			return "", false, true
		}
		if size > 2048 {
			return "", false, false
		}
		if size > uint64(len(payload)-at-header) {
			return "", false, true
		}
		value := payload[at+header : at+header+int(size)]
		at += header + int(size)
		if !utf8.Valid(value) {
			return "", false, false
		}
		return string(value), true, false
	}
	switch {
	case typ == 0x40, typ >= 0x5e && typ <= 0x77:
		name, ok, needMore := readString()
		if needMore {
			return false, true
		}
		if !ok {
			return false, false
		}
		return wcfBinaryXMLRootEvidence(name), wcfBinaryXMLRootEvidence(name)
	case typ == 0x41:
		prefix, ok, needMore := readString()
		if needMore {
			return false, true
		} else if !ok {
			return false, false
		}
		name, ok, needMore := readString()
		if needMore {
			return false, true
		}
		if !ok {
			return false, false
		}
		matched := wcfBinaryXMLRootEvidence(prefix) || wcfBinaryXMLRootEvidence(name)
		return matched, matched
	default:
		return false, false
	}
}

// Binary XML dictionary IDs are session-local, but literal element names and
// the standard WCF/SOAP namespace URIs remain observable. The MC-NMF record
// type/length plus a structurally complete element name is strong enough for
// midstream admission while the envelope itself remains opaque.
func wcfBinaryXMLRootEvidence(name string) bool {
	lower := strings.ToLower(name)
	if lower == "envelope" {
		return true
	}
	for _, marker := range []string{
		"http://tempuri.org/",
		"http://schemas.xmlsoap.org/soap/envelope/",
		"http://schemas.microsoft.com/",
		"http://schemas.datacontract.org/",
		"net.tcp://",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func wcfEnvelopePayloadEvidence(payload []byte) (complete bool, possible bool) {
	if complete, possible := wcfTextEnvelopePrefix(payload); complete || possible {
		return complete, possible
	}
	return wcfBinaryXMLEnvelopePrefix(payload)
}

func wcfProbeMidstream(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	typ := w[0]
	if typ == wcfRecordFault {
		rec, complete, err := parseWCFRecord(w, limit)
		if err != nil {
			return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}
		}
		if complete {
			if strings.HasPrefix(rec.text, wcfFaultNamespace) {
				return probeAccept("wcf", "midstream", 93)
			}
			return ProbeResult{Verdict: ProbeReject, Reason: "Fault URI is not an MC-NMF framing fault"}
		}
		if rec.headerLen > 0 && len(w) > 1+rec.headerLen {
			available := w[1+rec.headerLen:]
			namespace := []byte(wcfFaultNamespace)
			if bytes.HasPrefix(namespace, available) || bytes.HasPrefix(available, namespace) {
				return ProbeResult{Verdict: ProbeNeedMore, Protocol: "wcf", NeedBytes: max(1, rec.total-len(w)), Confidence: 75, Reason: "incomplete MC-NMF Fault"}
			}
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if typ != wcfRecordSizedEnvelope && typ != wcfRecordUnsizedEnvelope {
		return ProbeResult{Verdict: ProbeReject}
	}
	rec, complete, err := parseWCFRecord(w, limit)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}
	}
	if typ == wcfRecordSizedEnvelope {
		if len(w) < 2 {
			return ProbeResult{Verdict: ProbeReject}
		}
		if !complete {
			// The declared body length is already bounded. Hold only when the
			// available payload prefix is compatible with a SOAP/Binary XML root.
			if len(w) <= 1+rec.headerLen {
				return ProbeResult{Verdict: ProbeReject}
			}
			body := w[1+rec.headerLen:]
			possible := false
			if len(body) > 0 {
				_, possible = wcfEnvelopePayloadEvidence(body)
			}
			if !possible {
				return ProbeResult{Verdict: ProbeReject}
			}
			return ProbeResult{Verdict: ProbeNeedMore, Protocol: "wcf", NeedBytes: rec.total - len(w), Confidence: 70, Reason: "incomplete bounded envelope"}
		}
		if !complete {
			return ProbeResult{Verdict: ProbeReject}
		}
		ok, _ := wcfEnvelopePayloadEvidence(rec.body)
		if !ok {
			return ProbeResult{Verdict: ProbeReject, Reason: "no SOAP/Binary XML envelope root"}
		}
		return probeAccept("wcf", "midstream", 94)
	}
	if !complete {
		if len(rec.chunks) == 0 {
			return ProbeResult{Verdict: ProbeReject}
		}
		first := rec.chunks[0]
		body := w[first.offset:min(len(w), first.offset+int(first.size))]
		_, possible := wcfEnvelopePayloadEvidence(body)
		if !possible {
			return ProbeResult{Verdict: ProbeReject}
		}
		return ProbeResult{Verdict: ProbeNeedMore, Protocol: "wcf", NeedBytes: 1, Confidence: 65, Reason: "incomplete bounded unsized envelope"}
	}
	body := wcfRecordDeclaredBody(rec)
	ok, _ := wcfEnvelopePayloadEvidence(body)
	if !ok {
		return ProbeResult{Verdict: ProbeReject, Reason: "no SOAP/Binary XML envelope root"}
	}
	return probeAccept("wcf", "midstream", 92)
}

func probeWCF(w []byte, limit int) ProbeResult {
	if limit <= 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) > limit {
		w = w[:limit]
	}
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] == wcfRecordVersion {
		return wcfProbePreamble(w, limit)
	}
	return wcfProbeMidstream(w, limit)
}
