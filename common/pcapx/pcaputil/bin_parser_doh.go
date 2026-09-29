package pcaputil

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// binDoH is the M0 session state for RFC 8484 DNS-over-HTTPS on HTTP/1.1
// or HTTP/2 plaintext. Ports 443/53 are never consulted. TLS decryption
// is out of scope; Feed is the inner HTTP.
type binDoH struct {
	// Keys identify HTTP exchanges, not DNS IDs (RFC 8484 recommends ID 0).
	pending                             map[uint64]dohPending
	http1Requests, http1Responses       uint64
	flow                                *binFlow
	reserved, streamBytes, pendingBytes int64
	pendingSlots                        int
}

// Retain fixed-size question identity; display names must not define DNS
// equality (escaped labels and ASCII case have different representations).
type dohPending struct {
	name      string
	questions [32]byte
	opcode    uint16
}

func dohQuestionIdentity(questions []map[string]any) [32]byte {
	h := sha256.New()
	for _, q := range questions {
		var canonical [255]byte
		wire := q["Name Wire"].([]byte)
		for i, c := range wire {
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			canonical[i] = c
		}
		_, _ = h.Write(canonical[:len(wire)])
		var fields [4]byte
		binary.BigEndian.PutUint16(fields[:2], q["Type"].(uint16))
		binary.BigEndian.PutUint16(fields[2:], q["Class"].(uint16))
		_, _ = h.Write(fields[:])
	}
	var identity [32]byte
	copy(identity[:], h.Sum(nil))
	return identity
}

func dohMedia(v string) bool {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(v, ";", 2)[0]))
	return m == "application/dns-message"
}

func dohPathEvidence(path string) bool {
	queryAt := strings.IndexByte(path, '?')
	if queryAt < 0 {
		return false
	}
	if !strings.Contains(path[queryAt+1:], "dns") {
		return false
	}
	query, err := url.ParseQuery(path[queryAt+1:])
	if err != nil {
		return false
	}
	values, ok := query["dns"]
	if !ok || len(values) != 1 {
		return false
	}
	// The endpoint URI is configured out of band. A common /dns-query path
	// gives enough context to report a malformed query; on an arbitrary HTTP
	// path, require an actual DNS wire question to avoid claiming application
	// parameters such as /search?dns=example.com.
	if strings.HasSuffix(path[:queryAt], "/dns-query") {
		return true
	}
	msg, err := base64.RawURLEncoding.DecodeString(values[0])
	if err != nil || len(msg) < 12 || msg[2]&0x80 != 0 || binary.BigEndian.Uint16(msg[4:6]) == 0 {
		return false
	}
	_, _, err = dnsQuestion(msg)
	return err == nil
}

func dohHTTP1RequestEvidence(method, path, contentType string) bool {
	switch method {
	case http.MethodGet:
		return dohPathEvidence(path)
	case http.MethodPost:
		return dohMedia(contentType)
	default:
		return false
	}
}

func (f *binFlow) consumeDoHHTTP1(raw []byte) (map[string]any, error) {
	if bytes.HasPrefix(raw, []byte("HTTP/")) {
		return f.dohHTTP1Response(raw)
	}
	return f.dohHTTP1Request(raw)
}

func (f *binFlow) dohHTTP1Request(raw []byte) (map[string]any, error) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return nil, nil
	}
	defer req.Body.Close()
	ctype := req.Header.Get("Content-Type")
	path := req.URL.RequestURI()
	if !dohHTTP1RequestEvidence(req.Method, path, ctype) {
		return nil, nil
	}
	if err := f.ensureDoH(); err != nil {
		return nil, err
	}
	info := map[string]any{
		"DoH":                 true,
		"HTTP Version":        "1.1",
		"Method":              req.Method,
		"Path":                path,
		"Content Type":        ctype,
		"Protocol Transition": "http->doh",
		"Context Level":       "observed",
	}
	var msg []byte
	switch req.Method {
	case http.MethodGet:
		msg, err = dohDecodeGET(path)
		if err != nil {
			return info, err
		}
	case http.MethodPost:
		if !dohMedia(ctype) {
			return info, protocolError(ErrMalformedMessage, "DoH POST requires application/dns-message")
		}
		msg, err = io.ReadAll(io.LimitReader(req.Body, int64(f.a.config.MaxMessageBytes)+1))
		if err != nil {
			return info, err
		}
		if len(msg) > f.a.config.MaxMessageBytes {
			return info, protocolError(ErrResourceExceeded, "DoH POST body exceeds limit")
		}
	default:
		return info, protocolError(ErrUnsupportedFeature, "DoH method must be GET or POST")
	}
	return f.doh.attachDNS(info, msg, false, f.a.budget.MaxCollectionElements)
}

func (f *binFlow) dohHTTP1Response(raw []byte) (map[string]any, error) {
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), &http.Request{Method: http.MethodGet})
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	ctype := resp.Header.Get("Content-Type")
	if f.doh == nil && !dohMedia(ctype) {
		return nil, nil
	}
	if f.doh == nil {
		return nil, nil
	}
	info := map[string]any{
		"DoH":           true,
		"HTTP Version":  "1.1",
		"HTTP Status":   resp.StatusCode,
		"Content Type":  ctype,
		"Context Level": "observed",
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(f.a.config.MaxMessageBytes)+1))
	if err != nil {
		return info, err
	}
	if len(body) > f.a.config.MaxMessageBytes {
		return info, protocolError(ErrResourceExceeded, "DoH response body exceeds limit")
	}
	if resp.StatusCode >= 300 {
		f.doh.removePending(f.doh.responseKey(info))
		info["Packet Name"] = "Error"
		info["Error Body"] = bytes.Clone(body)
		info["Association Status"] = "http-error"
		return info, nil
	}
	if !dohMedia(ctype) {
		return info, protocolError(ErrMalformedMessage, "DoH response requires application/dns-message")
	}
	return f.doh.attachDNS(info, body, true, f.a.budget.MaxCollectionElements)
}

func (f *binFlow) consumeDoHH2(dir int, e *ProtocolEvent, stream *binH2Stream) (err error) {
	h := f.h2
	if h == nil || e.Session == nil {
		return nil
	}
	id, _ := e.Session["Stream ID"].(uint32)
	if typ, _ := e.Session["Frame Type"].(byte); typ == 7 && f.doh != nil && dir != h.client {
		last := h.lastAllowed[dir]
		for streamID, abandoned := range h.streams {
			if streamID > last && streamID&1 != 0 {
				f.doh.removePending(uint64(streamID))
				f.doh.clearStream(abandoned)
				abandoned.dohFailed = true
			}
		}
		return nil
	}
	if stream == nil {
		stream = h.streams[id]
	}
	if stream == nil {
		return nil
	}
	if e.Session["Reset"] == true {
		if f.doh != nil {
			f.doh.removePending(uint64(id))
		}
		if f.doh != nil {
			f.doh.clearStream(stream)
		}
		return nil
	}
	if stream.dohFailed {
		e.Session["DoH State"] = "invalid-message-stream"
		return nil
	}
	headers, _ := e.Session["Headers"].([]map[string]any)
	path, ctype, method := "", "", stream.method
	for _, header := range headers {
		name, _ := header["Name"].(string)
		value, _ := header["Value"].(string)
		switch name {
		case ":path":
			path = value
		case "content-type":
			ctype = value
		case ":method":
			method = value
		}
	}
	kind, _ := e.Session["Header Kind"].(string)
	if kind == "request" && dohHTTP1RequestEvidence(method, path, ctype) || kind == "response" && dohMedia(ctype) {
		stream.doh[dir] = true
	}
	if !stream.doh[0] && !stream.doh[1] {
		return nil
	}
	if err := f.ensureDoH(); err != nil {
		e.Session["DoH"], e.Session["Error Scope"] = true, "stream"
		stream.dohFailed, stream.doh = true, [2]bool{}
		return err
	}
	defer func() {
		if err != nil {
			// Application decoding failure does not invalidate HTTP/2 framing
			// or another stream's HPACK/DoH context.
			e.Session["Error Scope"] = "stream"
			stream.dohFailed = true
			f.doh.removePending(uint64(id))
			f.doh.clearStream(stream)
		} else if e.Session["Exchange Complete"] == true {
			f.doh.removePending(uint64(id))
			f.doh.clearStream(stream)
		} else {
			// All allocation paths reserve before mutation, so lowering to
			// the current retained size cannot fail.
			f.doh.syncStream(stream)
		}
	}()
	e.Session["DoH"] = true
	if stream.dohRetained == 0 {
		if err := f.doh.reserve(f.doh.retainedBytes() + 128); err != nil {
			return err
		}
		f.doh.syncStream(stream)
	}
	e.Session["HTTP Version"] = "2"
	e.Session["Protocol Transition"] = "http2->doh"
	if path != "" {
		e.Session["Path"] = path
	}
	if method != "" {
		e.Session["Method"] = method
	}
	if kind == "request" || kind == "response" {
		// Retain only the bounded media type, not arbitrary parameters or a
		// substring that keeps the entire decoded header value alive.
		media := strings.TrimSpace(strings.SplitN(ctype, ";", 2)[0])
		if len(media) <= 255 {
			if err := f.doh.reserve(f.doh.retainedBytes() + int64(2*len(media))); err != nil {
				return err
			}
			stream.dohContentType[dir] = strings.ToLower(strings.Clone(media))
			f.doh.syncStream(stream)
		}
	}
	if kind == "response" {
		status, _ := dohPseudo(headers, ":status")
		if err := f.doh.reserve(f.doh.retainedBytes() + int64(len(status))); err != nil {
			return err
		}
		stream.dohStatus[dir] = strings.Clone(status)
		f.doh.syncStream(stream)
	}
	if ctype = stream.dohContentType[dir]; ctype != "" {
		e.Session["Content Type"] = ctype
	}
	status := stream.dohStatus[dir]
	if status != "" {
		e.Session["HTTP Status"] = status
	}
	httpResp := dir != h.client
	httpError := httpResp && status != "" && status[0] != '2'
	if httpError {
		e.Session["Packet Name"] = "Error"
		e.Session["Association Status"] = "http-error"
	}
	if kind == "request" && method == http.MethodGet {
		msg, decodeErr := dohDecodeGET(path)
		if decodeErr != nil {
			return decodeErr
		}
		_, err = f.doh.attachDNS(e.Session, msg, false, f.a.budget.MaxCollectionElements)
		return err
	}
	typ, _ := e.Session["Frame Type"].(byte)
	if typ == 0 {
		payload := grpcDATAPayload(e.Raw)
		old := stream.dohBuf[dir]
		size := len(old) + len(payload)
		if size > f.a.config.MaxMessageBytes {
			return protocolError(ErrResourceExceeded, "DoH buffered DATA exceeds limit")
		}
		if size <= cap(old) {
			stream.dohBuf[dir] = append(old, payload...)
		} else {
			capacity := max(size, min(2*cap(old), f.a.config.MaxMessageBytes))
			// The old backing array remains live while copying into the new
			// one. Reserve their sum, not just the eventual capacity delta.
			if err := f.doh.reserve(f.doh.retainedBytes() + int64(capacity)); err != nil {
				return err
			}
			buf := make([]byte, size, capacity)
			copy(buf, old)
			copy(buf[len(old):], payload)
			stream.dohBuf[dir] = buf
			old = nil
			f.doh.syncStream(stream)
		}
	}
	// END_STREAM can be carried by DATA, empty response HEADERS, or
	// trailers. Keep status and media type from the initial response HEADERS.
	if e.Session["End Stream"] == true && (typ == 0 || kind == "request" || kind == "response" || kind == "trailers") {
		buf := stream.dohBuf[dir]
		// Keep the buffer charged through DNS decoding; the deferred stream
		// synchronization releases it only after this function returns.
		defer func() { stream.dohBuf[dir] = nil }()
		if httpError {
			f.doh.removePending(uint64(id))
			e.Session["Error Body"] = buf // transfer owned storage to the event
			return nil
		}
		if httpResp && !dohMedia(ctype) {
			return protocolError(ErrUnsupportedFeature, "DoH response media type is not application/dns-message")
		}
		if !httpResp && method == http.MethodGet {
			return nil // GET's DNS message was already decoded from the URI.
		}
		_, err = f.doh.attachDNS(e.Session, buf, httpResp, f.a.budget.MaxCollectionElements)
		return err
	}
	return nil
}

func dohPseudo(headers []map[string]any, name string) (string, bool) {
	for _, header := range headers {
		n, _ := header["Name"].(string)
		if n == name {
			v, _ := header["Value"].(string)
			return v, true
		}
	}
	return "", false
}

func dohDecodeGET(path string) ([]byte, error) {
	i := strings.Index(path, "?")
	if i < 0 {
		return nil, protocolError(ErrMalformedMessage, "DoH GET missing dns parameter")
	}
	query, err := url.ParseQuery(path[i+1:])
	if err != nil {
		return nil, protocolError(ErrMalformedMessage, "DoH GET query is malformed")
	}
	values := query["dns"]
	if len(values) != 1 || values[0] == "" {
		return nil, protocolError(ErrMalformedMessage, "DoH GET requires exactly one dns parameter")
	}
	msg, err := base64.RawURLEncoding.DecodeString(values[0])
	if err != nil {
		return nil, protocolError(ErrMalformedMessage, "DoH GET dns parameter is not base64url")
	}
	return msg, nil
}

func (f *binFlow) ensureDoH() error {
	if f.doh == nil {
		d := &binDoH{flow: f}
		if err := d.reserve(d.retainedBytes()); err != nil {
			return err
		}
		f.doh = d
	}
	return nil
}

func (d *binDoH) attachDNS(info map[string]any, msg []byte, response bool, max int) (map[string]any, error) {
	semantic, err := DecodeDNSMessage(msg, max)
	if err != nil {
		return info, err
	}
	info["DNS"] = semantic
	if len(msg) < 12 {
		return info, protocolError(ErrMalformedMessage, "DoH DNS message shorter than header")
	}
	id := binary.BigEndian.Uint16(msg[0:2])
	flags := binary.BigEndian.Uint16(msg[2:4])
	if (flags&0x8000 != 0) != response {
		return info, protocolError(ErrMalformedMessage, "DoH DNS QR contradicts HTTP direction")
	}
	questions := semantic["Questions"].([]map[string]any)
	var qname string
	var qtype uint16
	if len(questions) > 0 {
		qname = questions[0]["Name"].(string)
		qtype = questions[0]["Type"].(uint16)
		info["QCLASS"] = questions[0]["Class"]
	}
	pending := dohPending{name: qname, questions: dohQuestionIdentity(questions), opcode: (flags >> 11) & 15}
	info["Transaction ID"] = id
	info["QR"] = flags>>15 != 0
	info["Opcode"] = (flags >> 11) & 0xf
	// RFC 6891: OPT TTL's high octet supplies the upper eight RCODE bits.
	// Use the already-decoded Additional records; a header RCODE of zero
	// alone does not mean success (for example, BADVERS is RCODE 16).
	rcode := flags & 0xf
	for _, rr := range semantic["Additional"].([]map[string]any) {
		if rr["Type"] == uint16(41) {
			rcode |= uint16(rr["TTL"].(uint32)>>24) << 4
			break
		}
	}
	info["RCODE"], semantic["RCODE"] = rcode, rcode
	info["QNAME"] = qname
	info["QTYPE"] = qtype
	info["QTYPE Name"] = dnsTypeName(qtype)
	if !response {
		info["Packet Name"] = "Query"
		// An HTTP/2 server can reject a POST before the upload finishes.
		// Its later request DATA must not recreate an already-ended exchange.
		if info["Exchange Complete"] == true {
			info["Outstanding"] = false
			return info, nil
		}
		if max <= 0 {
			max = 4096
		}
		if len(d.pending) >= max {
			return info, protocolError(ErrResourceExceeded, "DoH outstanding HTTP exchanges exceed budget")
		}
		key := uint64(0)
		if stream, ok := info["Stream ID"].(uint32); ok {
			key = uint64(stream)
		} else {
			d.http1Requests++
			key = d.http1Requests
		}
		if err := d.addPending(key, pending); err != nil {
			return info, err
		}
		info["Outstanding"] = true
		return info, nil
	}
	info["Packet Name"] = "Response"
	key := d.responseKey(info)
	if want, ok := d.pending[key]; ok {
		d.removePending(key)
		// Error replies may omit a question they could not interpret. The HTTP
		// exchange still identifies them; do not invent a response question.
		omittedErrorQuestion := len(questions) == 0 && rcode != 0
		if want.opcode != pending.opcode || (!omittedErrorQuestion && want.questions != pending.questions) {
			info["Association Status"] = "question-mismatch"
			return info, protocolError(ErrMalformedMessage, "DoH DNS response question contradicts HTTP request")
		}
		info["Matched Request"] = want.name
		if omittedErrorQuestion {
			info["Question Status"] = "omitted-error-question"
		}
		info["Association Status"] = "matched"
	} else {
		info["Unmatched"] = true
		info["Association Status"] = "missing-request"
		info["Context Level"] = "partial"
	}
	if addrs := dnsARecords(msg); len(addrs) > 0 {
		info["A Records"] = addrs
	}
	return info, nil
}

// HTTP/1.1 responses have the order of the observed DoH requests; HTTP/2
// exchanges use the stream ID, independently of arrival order and DNS ID.
func (d *binDoH) responseKey(info map[string]any) uint64 {
	if stream, ok := info["Stream ID"].(uint32); ok {
		return uint64(stream)
	}
	d.http1Responses++
	return d.http1Responses
}

func dohSummary(session map[string]any) string {
	return fmt.Sprintf("DoH %v %v %v", session["Method"], session["Packet Name"], session["QNAME"])
}

// DoH uses a separate shrinking reservation because reserveSession retains a
// high-water mark for other HTTP/H2 state. Charging both through a.buffered keeps
// the capture limit global without releasing another decoder's reservation.
func (d *binDoH) retainedBytes() int64 {
	return 256 + d.streamBytes + d.pendingBytes
}

func (d *binDoH) reserve(target int64) error {
	if d.flow == nil {
		return nil
	}
	delta := target - d.reserved
	a := d.flow.a
	if delta <= 0 {
		a.buffered.Add(delta)
		d.reserved = target
		return nil
	}
	for {
		current := a.buffered.Load()
		if delta > int64(a.config.MaxBufferedBytes)-current {
			return protocolError(ErrResourceExceeded, "DoH retained state exceeds capture memory budget")
		}
		if a.buffered.CompareAndSwap(current, current+delta) {
			d.reserved = target
			for peak := a.peak.Load(); current+delta > peak; peak = a.peak.Load() {
				if a.peak.CompareAndSwap(peak, current+delta) {
					break
				}
			}
			return nil
		}
	}
}

func (d *binDoH) syncStream(s *binH2Stream) {
	var size int64
	if s.doh[0] || s.doh[1] {
		// Additional DoH state and bookkeeping beyond HTTP/2's base stream
		// estimate, plus every retained backing-array capacity and string.
		size = 128
		for dir := range s.dohBuf {
			size += int64(cap(s.dohBuf[dir]) + len(s.dohContentType[dir]) + len(s.dohStatus[dir]))
		}
	}
	d.streamBytes += size - s.dohRetained
	s.dohRetained = size
	// Growth is pre-reserved by the caller; this call releases spare or
	// temporary copying storage after the owning references have changed.
	_ = d.reserve(d.retainedBytes())
}

func (d *binDoH) clearStream(s *binH2Stream) {
	s.dohBuf = [2][]byte{}
	s.dohContentType, s.dohStatus = [2]string{}, [2]string{}
	s.doh = [2]bool{}
	d.syncStream(s)
}

func (d *binDoH) addPending(key uint64, pending dohPending) error {
	name := pending.name
	old, exists := d.pending[key]
	slots := d.pendingSlots
	if !exists && len(d.pending)+1 > slots {
		slots = len(d.pending) + 1
	}
	// The map keeps buckets after deletion. Charge a conservative slot high
	// water until the map is empty and its backing storage can be discarded.
	next := d.pendingBytes + int64(len(name)-len(old.name)+(slots-d.pendingSlots)*96)
	if d.pending == nil {
		next += 128
	}
	if err := d.reserve(d.retainedBytes() - d.pendingBytes + next + int64(len(old.name))); err != nil {
		return err
	}
	if d.pending == nil {
		d.pending = make(map[uint64]dohPending)
	}
	pending.name = strings.Clone(name)
	d.pending[key] = pending
	d.pendingSlots, d.pendingBytes = slots, next
	return d.reserve(d.retainedBytes())
}

func (d *binDoH) removePending(key uint64) {
	if name, ok := d.pending[key]; ok {
		delete(d.pending, key)
		d.pendingBytes -= int64(len(name.name))
		if len(d.pending) == 0 {
			d.pending = nil
			d.pendingSlots, d.pendingBytes = 0, 0
		}
		_ = d.reserve(d.retainedBytes())
	}
}

func (f *binFlow) releaseDoH() {
	if d := f.doh; d != nil {
		if f.h2 != nil {
			for _, stream := range f.h2.streams {
				stream.dohBuf = [2][]byte{}
				stream.dohContentType, stream.dohStatus = [2]string{}, [2]string{}
				stream.dohRetained = 0
			}
		}
		d.pending = nil
		d.pendingSlots, d.pendingBytes, d.streamBytes = 0, 0, 0
		_ = d.reserve(0)
	}
}

// Generic HTTP/2 body/gRPC validation can fail before consumeDoHH2 runs.
// Release application storage even if H2 has already retired this stream.
func (f *binFlow) failDoHH2Stream(stream *binH2Stream, id uint32) {
	if f.doh == nil || stream == nil {
		return
	}
	f.doh.removePending(uint64(id))
	f.doh.clearStream(stream)
	stream.dohFailed = true
}
