package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type objectObservation struct {
	id, responseTo                                  uint64
	protocol, source, destination, uri, contentType string
	objectType, direction                           string
	method                                          string
	status                                          int
	contentEncoding, contentDisposition             string
	contentRange, suggestedFilename                 string
	authorization, cookie                           any
	transferID                                      uint64
	opcode                                          string
	request                                         bool
	allowEmpty                                      bool
	compressed                                      bool
	body                                            io.Reader
	declared                                        *int64
	refs                                            []pcaputil.PacketReference
	bytes                                           pcaputil.ByteSource
	httpStream                                      *httpStreamFragment
	streamID                                        uint64
}

type httpStreamKey struct {
	protocol         string
	domain           pcaputil.CaptureDomain
	flowID, streamID uint64
}

type httpStreamFragment struct {
	key        httpStreamKey
	direction  int
	headerKind string
	endStream  bool
	payload    []byte
	reset      bool
}

func observedObject(e *pcaputil.ProtocolEvent) *objectObservation {
	observations := observedObjects(e)
	if len(observations) == 0 {
		return nil
	}
	return observations[0]
}

func observedObjects(e *pcaputil.ProtocolEvent) []*objectObservation {
	if e == nil {
		return nil
	}
	if e.Protocol == "http3" {
		return observedHTTP3Objects(e)
	}
	if observation := observedHTTP2Object(e); observation != nil {
		return []*objectObservation{observation}
	}
	if observation := observedHTTP1Object(e); observation != nil {
		return []*objectObservation{observation}
	}
	if observations := observedWebSocketObjects(e); len(observations) > 0 {
		return observations
	}
	if observation := observedTFTPObject(e); observation != nil {
		return []*objectObservation{observation}
	}
	return nil
}

func observedHTTP1Object(e *pcaputil.ProtocolEvent) *objectObservation {
	body, declared, bodyRequest, bodyOK := e.HTTPMessageBody()
	snapshot := e.DisplayFieldValues()
	fields := snapshot.FieldMap()
	request := len(fields["http.request.method"]) > 0
	if bodyOK {
		request = bodyRequest
	}
	if !request && !bodyOK {
		return nil
	}
	if !bodyOK && (!request || e.Completeness != "message" && e.Completeness != "transaction") {
		return nil
	}
	literal := func(key string) any {
		values := fields[key]
		if len(values) == 0 {
			return nil
		}
		if len(values) == 1 {
			return values[0]
		}
		// Several literal occurrences must not be silently rewritten or joined.
		return append([]any(nil), values...)
	}
	text := func(key string) string {
		if values := fields[key]; len(values) > 0 {
			value, _ := values[0].(string)
			return value
		}
		return ""
	}
	status := 0
	if values := fields["http.response.code"]; len(values) > 0 {
		if value, ok := objectUint64(values[0]); ok {
			status = int(value)
		}
	}
	observation := &objectObservation{id: e.ID, responseTo: e.ResponseTo, protocol: e.Protocol, objectType: "http_body", source: e.Source, destination: e.Destination, request: request, body: body, declared: declared, method: text("http.request.method"), status: status,
		uri: text("http.request.uri"), authorization: literal("http.request.authorization"), cookie: literal("http.cookie"), refs: snapshot.Source.PacketRefs, bytes: snapshot.Source}
	applyObjectMessageMetadata(observation, func(name string) []string { return e.HTTPMessageHeaderValues(name) })
	return observation
}

func observedHTTP2Object(e *pcaputil.ProtocolEvent) *objectObservation {
	if e == nil || e.Protocol != "http2" && e.Protocol != "grpc" {
		return nil
	}
	key := httpStreamKey{protocol: "h2", domain: e.Domain, flowID: e.TCPFlowID}
	if reset, ok := e.HTTP2StreamReset(); ok {
		key.streamID = reset
		return &objectObservation{id: e.ID, protocol: e.Protocol, httpStream: &httpStreamFragment{key: key, reset: true}}
	}
	if data, ok := e.HTTP2DataFragment(); ok {
		key.streamID = data.StreamID
		snapshot := e.DisplayFieldValues()
		return &objectObservation{id: e.ID, protocol: e.Protocol, refs: snapshot.Source.PacketRefs, bytes: snapshot.Source,
			httpStream: &httpStreamFragment{key: key, direction: e.Direction, endStream: data.EndStream, payload: data.Payload}}
	}
	header, ok := e.HTTP2HeaderFragment()
	if !ok {
		return nil
	}
	key.streamID = header.StreamID
	snapshot := e.DisplayFieldValues()
	fields := snapshot.FieldMap()
	text := func(key string) string {
		if values := fields[key]; len(values) > 0 {
			value, _ := values[0].(string)
			return value
		}
		return ""
	}
	headerLiteral := func(name string) any {
		values := e.HTTPMessageHeaderValues(name)
		if len(values) == 0 {
			return nil
		}
		if len(values) == 1 {
			return values[0]
		}
		out := make([]any, 0, len(values))
		for _, value := range values {
			out = append(out, value)
		}
		return out
	}
	var declared *int64
	if values := e.HTTPMessageHeaderValues("Content-Length"); len(values) > 0 {
		if length, err := strconv.ParseInt(strings.TrimSpace(values[0]), 10, 64); err == nil && length >= 0 {
			declared = &length
		}
	}
	status := 0
	if values := fields["http.response.code"]; len(values) > 0 {
		if value, ok := objectUint64(values[0]); ok {
			status = int(value)
		}
	}
	observation := &objectObservation{id: e.ID, protocol: e.Protocol, objectType: "http_body", source: e.Source, destination: e.Destination, request: header.Kind == "request", method: text("http.request.method"), status: status,
		uri: text("http.request.uri"), declared: declared, authorization: headerLiteral("Authorization"), cookie: headerLiteral("Cookie"),
		refs: snapshot.Source.PacketRefs, bytes: snapshot.Source,
		httpStream: &httpStreamFragment{key: key, direction: e.Direction, headerKind: header.Kind, endStream: header.EndStream}}
	applyObjectMessageMetadata(observation, func(name string) []string { return e.HTTPMessageHeaderValues(name) })
	return observation
}

func observedHTTP3Objects(e *pcaputil.ProtocolEvent) []*objectObservation {
	if e == nil || e.Protocol != "http3" || e.Status == "malformed" || e.Session == nil {
		return nil
	}
	snapshot := e.DisplayFieldValues()
	frames := http3ObjectFrames(e)
	completions := map[httpStreamKey]bool{}
	var out []*objectObservation
	if quicFrames, ok := e.Session["Frames"].([]map[string]any); ok {
		for _, frame := range quicFrames {
			if frame["Frame Type"] != "RESET_STREAM" {
				continue
			}
			streamID, ok := objectUint64(frame["Stream ID"])
			if !ok {
				continue
			}
			key := httpStreamKey{protocol: "h3", domain: e.Domain, flowID: e.FlowID, streamID: streamID}
			out = append(out, &objectObservation{id: e.ID, protocol: e.Protocol, source: e.Source, destination: e.Destination,
				httpStream: &httpStreamFragment{key: key, reset: true}})
		}
	}
	addCompletion := func(values map[string]any) {
		if values == nil || values["Drained FIN"] != true {
			return
		}
		streamID, streamOK := objectUint64(values["Stream ID"])
		direction, directionOK := objectInt(values["Direction"])
		if !streamOK || !directionOK || direction != 0 && direction != 1 {
			return
		}
		key := httpStreamKey{protocol: "h3", domain: e.Domain, flowID: e.FlowID, streamID: streamID}
		if completions[key] {
			return
		}
		completions[key] = true
		out = append(out, &objectObservation{id: e.ID, protocol: e.Protocol, source: e.Source, destination: e.Destination,
			httpStream: &httpStreamFragment{key: key, direction: direction, endStream: true}})
	}
	for _, frame := range frames {
		streamID, streamOK := objectUint64(frame["Stream ID"])
		direction, directionOK := objectInt(frame["Direction"])
		if !streamOK || !directionOK || direction != 0 && direction != 1 {
			continue
		}
		key := httpStreamKey{protocol: "h3", domain: e.Domain, flowID: e.FlowID, streamID: streamID}
		switch frame["Frame Type"] {
		case "HEADERS":
			kind, _ := frame["Header Kind"].(string)
			if kind != "request" && kind != "response" {
				continue
			}
			headers, _ := frame["Headers"].([]map[string]any)
			out = append(out, http3HeaderObject(e, key, direction, kind, headers, snapshot))
		case "DATA":
			payload, ok := frame["Payload"].([]byte)
			if !ok {
				continue
			}
			out = append(out, &objectObservation{id: e.ID, protocol: e.Protocol, refs: snapshot.Source.PacketRefs, bytes: snapshot.Source,
				httpStream: &httpStreamFragment{key: key, direction: direction, payload: bytes.Clone(payload)}})
		}
		if conformance, ok := frame["Message Conformance"].(map[string]any); ok {
			addCompletion(conformance)
		}
	}
	if conformance, ok := e.Session["HTTP3 Message Conformance"].(map[string]any); ok {
		addCompletion(conformance)
	}
	return out
}

func http3ObjectFrames(e *pcaputil.ProtocolEvent) []map[string]any {
	found := false
	var out []map[string]any
	add := func(value any) {
		frames, ok := value.([]map[string]any)
		if !ok {
			return
		}
		found = found || len(frames) > 0
		out = append(out, frames...)
	}
	if frames, ok := e.Session["Frames"].([]map[string]any); ok {
		for _, frame := range frames {
			add(frame["HTTP3 Frames"])
		}
	}
	if resumed, ok := e.Session["QPACK Resumed Streams"].([]map[string]any); ok {
		for _, stream := range resumed {
			add(stream["HTTP3 Frames"])
		}
	}
	if found {
		return out
	}
	frames, _ := e.Session["HTTP3 Frames"].([]map[string]any)
	return frames
}

func http3HeaderObject(e *pcaputil.ProtocolEvent, key httpStreamKey, direction int, kind string, headers []map[string]any, snapshot pcaputil.DisplayFieldSnapshot) *objectObservation {
	values := func(name string) []string {
		var out []string
		for _, header := range headers {
			field, _ := header["Name"].(string)
			value, _ := header["Value"].(string)
			if strings.EqualFold(field, name) {
				out = append(out, value)
			}
		}
		return out
	}
	first := func(name string) string {
		if out := values(name); len(out) > 0 {
			return out[0]
		}
		return ""
	}
	literal := func(name string) any {
		values := values(name)
		if len(values) == 0 {
			return nil
		}
		if len(values) == 1 {
			return values[0]
		}
		out := make([]any, 0, len(values))
		for _, value := range values {
			out = append(out, value)
		}
		return out
	}
	var declared *int64
	if value := first("Content-Length"); value != "" {
		if length, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && length >= 0 {
			declared = &length
		}
	}
	status := 0
	if value := first(":status"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			status = parsed
		}
	}
	observation := &objectObservation{id: e.ID, protocol: e.Protocol, objectType: "http_body", source: e.Source, destination: e.Destination, request: kind == "request",
		method: first(":method"), uri: first(":path"), status: status, declared: declared,
		authorization: literal("Authorization"), cookie: literal("Cookie"), refs: snapshot.Source.PacketRefs, bytes: snapshot.Source,
		streamID: key.streamID, httpStream: &httpStreamFragment{key: key, direction: direction, headerKind: kind}}
	applyObjectMessageMetadata(observation, values)
	return observation
}

func applyObjectMessageMetadata(o *objectObservation, values func(string) []string) {
	first := func(name string) string {
		if out := values(name); len(out) > 0 {
			return out[0]
		}
		return ""
	}
	o.contentType = first("Content-Type")
	o.contentEncoding = strings.Join(values("Content-Encoding"), ", ")
	o.contentRange = first("Content-Range")
	o.contentDisposition = first("Content-Disposition")
	if o.contentDisposition != "" {
		if _, parameters, err := mime.ParseMediaType(o.contentDisposition); err == nil {
			if filename := parameters["filename"]; filename != "" {
				o.suggestedFilename = safeObjectName(filename, o.contentType)
			}
		}
	}
}

func objectUint64(value any) (uint64, bool) {
	switch value := value.(type) {
	case uint64:
		return value, true
	case uint32:
		return uint64(value), true
	case uint:
		return uint64(value), true
	case int:
		return uint64(value), value >= 0
	case int64:
		return uint64(value), value >= 0
	default:
		return 0, false
	}
}

func objectInt(value any) (int, bool) {
	number, ok := objectUint64(value)
	return int(number), ok
}

type objectExporter struct {
	directory       string
	requests        map[uint64]*objectObservation
	requestBytes    int
	httpStream      map[httpStreamKey]*httpStreamState
	httpStreamBytes int
}

type httpStreamDirection struct {
	header   *objectObservation
	declared *int64
	selected bool
	done     bool
	body     []byte
	refs     []pcaputil.PacketReference
	parents  []uint64
}

type httpStreamState struct {
	directions [2]*httpStreamDirection
}

func newObjectExporter(directory string) (*objectExporter, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create object directory: %w", err)
	}
	return &objectExporter{directory: directory, requests: make(map[uint64]*objectObservation), httpStream: make(map[httpStreamKey]*httpStreamState)}, nil
}

func (x *objectExporter) observe(observations []*objectObservation) error {
	for _, observation := range observations {
		if observation.httpStream != nil {
			continue
		}
		if !observation.request {
			continue
		}
		if _, exists := x.requests[observation.id]; exists {
			continue
		}
		cost := len(observation.uri) + len(fmt.Sprint(observation.authorization)) + len(fmt.Sprint(observation.cookie)) + 256
		if len(x.requests) >= 4096 || x.requestBytes+cost > 4<<20 {
			return fmt.Errorf("object export request context budget exceeded (4096 pending requests / 4 MiB)")
		}
		copy := *observation
		copy.body = nil
		copy.declared = nil
		copy.refs = nil
		copy.bytes = pcaputil.ByteSource{}
		x.requests[observation.id] = &copy
		x.requestBytes += cost
	}
	return nil
}

func (x *objectExporter) forget(observation *objectObservation) {
	if observation.httpStream != nil {
		return
	}
	if request := x.requests[observation.responseTo]; request != nil {
		x.requestBytes -= len(request.uri) + len(fmt.Sprint(request.authorization)) + len(fmt.Sprint(request.cookie)) + 256
		delete(x.requests, observation.responseTo)
	}
}

func safeObjectName(uri, contentType string) string {
	name := ""
	if u, err := url.Parse(uri); err == nil {
		name = path.Base(strings.ReplaceAll(u.Path, "\\", "/"))
	}
	if name == "." || name == "/" || name == ".." {
		name = ""
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	name = strings.Trim(name, ". ")
	for len(name) > 120 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	if name == "" {
		name = "object"
	}
	if windowsDeviceName(name) {
		name = "_" + name
	}
	if path.Ext(name) == "" {
		media, _, _ := mime.ParseMediaType(contentType)
		extensions := map[string]string{"application/octet-stream": ".bin", "text/plain": ".txt", "text/html": ".html", "application/json": ".json", "image/png": ".png", "image/jpeg": ".jpg", "application/pdf": ".pdf", "application/zip": ".zip"}
		if ext := extensions[media]; ext != "" {
			name += ext
		} else {
			name += ".bin"
		}
	}
	return name
}

func windowsDeviceName(name string) bool {
	base := name
	if at := strings.IndexByte(base, '.'); at >= 0 {
		base = base[:at]
	}
	base = strings.ToUpper(strings.TrimRight(base, " ."))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return true
	}
	return false
}

type objectManifest struct {
	RecordType         string              `json:"record_type"`
	Physical           bool                `json:"physical"`
	PDU                uint64              `json:"pdu"`
	Protocol           string              `json:"protocol"`
	ObjectType         string              `json:"object_type"`
	StreamID           uint64              `json:"stream_id,omitempty"`
	TransferID         uint64              `json:"transfer_id,omitempty"`
	Direction          string              `json:"direction"`
	Method             string              `json:"method,omitempty"`
	StatusCode         int                 `json:"status_code,omitempty"`
	Opcode             string              `json:"opcode,omitempty"`
	Compressed         bool                `json:"compressed,omitempty"`
	Source             string              `json:"source"`
	Destination        string              `json:"destination"`
	URI                any                 `json:"uri"`
	ContentType        string              `json:"content_type"`
	ContentEncoding    string              `json:"content_encoding,omitempty"`
	ContentDisposition string              `json:"content_disposition,omitempty"`
	ContentRange       string              `json:"content_range,omitempty"`
	SuggestedFilename  string              `json:"suggested_filename,omitempty"`
	Partial            bool                `json:"partial,omitempty"`
	DeclaredLength     *int64              `json:"declared_length"`
	ObservedLength     int64               `json:"observed_length"`
	Complete           bool                `json:"complete"`
	LengthMismatch     bool                `json:"length_mismatch,omitempty"`
	SHA256             string              `json:"sha256"`
	Path               string              `json:"path"`
	PacketRefs         []uint64            `json:"packet_refs"`
	ByteSource         pcaputil.ByteSource `json:"byte_source"`
	Authorization      any                 `json:"authorization"`
	Cookie             any                 `json:"cookie"`
}

func (x *objectExporter) process(o *objectObservation, matched bool) (*objectManifest, error) {
	if o.httpStream != nil {
		return x.processHTTPStream(o, matched)
	}
	if !matched {
		return nil, nil
	}
	return x.export(o)
}

func (x *objectExporter) export(o *objectObservation) (*objectManifest, error) {
	if o.body == nil {
		return nil, nil
	}
	var uri, authorization, cookie any
	method := ""
	uriText := ""
	if o.httpStream != nil {
		uriText, uri, authorization, cookie, method = o.uri, o.uri, o.authorization, o.cookie, o.method
	} else if o.request {
		uriText, uri, authorization, cookie, method = o.uri, o.uri, o.authorization, o.cookie, o.method
	} else if request := x.requests[o.responseTo]; request != nil {
		uriText = request.uri
		uri, authorization, cookie = request.uri, request.authorization, request.cookie
		method = request.method
	}
	nameText := uriText
	if o.suggestedFilename != "" {
		nameText = o.suggestedFilename
	}
	name := filepath.Join(x.directory, fmt.Sprintf("%d-%s", o.id, safeObjectName(nameText, o.contentType)))
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("create exported object: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), o.body)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(name)
		if copyErr != nil {
			return nil, fmt.Errorf("write object: %w", copyErr)
		}
		return nil, closeErr
	}
	if written == 0 && !o.allowEmpty {
		if err := os.Remove(name); err != nil {
			return nil, fmt.Errorf("remove empty exported body: %w", err)
		}
		return nil, nil
	}
	mismatch := o.declared != nil && *o.declared != written
	refs := make([]uint64, 0, len(o.refs))
	for _, ref := range o.refs {
		refs = append(refs, ref.Number)
	}
	direction := "response"
	if o.request {
		direction = "request"
	}
	if o.direction != "" {
		direction = o.direction
	}
	objectType := o.objectType
	if objectType == "" {
		objectType = "object"
	}
	return &objectManifest{RecordType: "object_export", PDU: o.id, Protocol: o.protocol, ObjectType: objectType, StreamID: o.streamID, TransferID: o.transferID, Direction: direction, Method: method, StatusCode: o.status, Opcode: o.opcode, Compressed: o.compressed, Source: o.source, Destination: o.destination, URI: uri, ContentType: o.contentType,
		ContentEncoding: o.contentEncoding, ContentDisposition: o.contentDisposition, ContentRange: o.contentRange, SuggestedFilename: o.suggestedFilename, Partial: o.contentRange != "",
		DeclaredLength: o.declared, ObservedLength: written, Complete: !mismatch, LengthMismatch: mismatch, SHA256: hex.EncodeToString(hash.Sum(nil)), Path: name, PacketRefs: refs, ByteSource: o.bytes, Authorization: authorization, Cookie: cookie}, nil
}

const maxPendingHTTPStreamBytes = 64 << 20

func (x *objectExporter) processHTTPStream(o *objectObservation, matched bool) (*objectManifest, error) {
	fragment := o.httpStream
	key := fragment.key
	if fragment.reset {
		if stream := x.httpStream[key]; stream != nil {
			x.releaseHTTPStreamBodies(stream)
			delete(x.httpStream, key)
		}
		return nil, nil
	}
	if fragment.direction != 0 && fragment.direction != 1 {
		return nil, fmt.Errorf("HTTP object export has invalid direction %d", fragment.direction)
	}
	stream := x.httpStream[key]
	if stream == nil {
		if len(x.httpStream) >= 4096 {
			return nil, fmt.Errorf("HTTP object export stream budget exceeded (4096 pending streams)")
		}
		stream = &httpStreamState{}
		x.httpStream[key] = stream
	}
	direction := stream.directions[fragment.direction]
	if direction == nil {
		direction = &httpStreamDirection{}
		stream.directions[fragment.direction] = direction
	}
	if fragment.headerKind != "" {
		if direction.header != nil && direction.done {
			return nil, nil
		}
		if direction.header == nil {
			copy := *o
			copy.body = nil
			copy.refs = nil
			copy.bytes = pcaputil.ByteSource{}
			direction.header = &copy
			declared := o.declared
			direction.declared = declared
			direction.selected = matched
			direction.done = fragment.endStream
			x.cleanupHTTPStream(key, stream)
		}
		return nil, nil
	}
	if direction.header == nil {
		return nil, nil
	}
	if matched {
		direction.selected = true
	}
	if len(fragment.payload) > 0 {
		if x.httpStreamBytes+len(fragment.payload) > maxPendingHTTPStreamBytes {
			return nil, fmt.Errorf("HTTP object export body budget exceeded (%d MiB)", maxPendingHTTPStreamBytes>>20)
		}
		direction.body = append(direction.body, fragment.payload...)
		x.httpStreamBytes += len(fragment.payload)
	}
	for _, ref := range o.refs {
		if !containsPacketReference(direction.refs, ref) {
			direction.refs = append(direction.refs, ref)
		}
	}
	if !containsUint64(direction.parents, o.id) {
		direction.parents = append(direction.parents, o.id)
	}
	if !fragment.endStream {
		return nil, nil
	}
	direction.done = true
	if !direction.selected || len(direction.body) == 0 {
		x.httpStreamBytes -= len(direction.body)
		direction.body = nil
		x.cleanupHTTPStream(key, stream)
		return nil, nil
	}

	body := direction.body
	bodyObservation := *direction.header
	bodyObservation.id = o.id
	bodyObservation.body = bytes.NewReader(body)
	bodyObservation.declared = direction.declared
	bodyObservation.refs = append([]pcaputil.PacketReference(nil), direction.refs...)
	bodyObservation.bytes = httpStreamBodyByteSource(body, direction.parents, direction.refs)
	bodyObservation.streamID = key.streamID
	if !bodyObservation.request {
		request := stream.directions[1-fragment.direction]
		if request != nil && request.header != nil {
			bodyObservation.uri = request.header.uri
			bodyObservation.authorization = request.header.authorization
			bodyObservation.cookie = request.header.cookie
			bodyObservation.method = request.header.method
		}
	}
	if !bodyObservation.request && bodyObservation.method == "CONNECT" && bodyObservation.status >= 200 && bodyObservation.status < 300 {
		x.httpStreamBytes -= len(body)
		direction.body = nil
		x.cleanupHTTPStream(key, stream)
		return nil, nil
	}
	manifest, err := x.export(&bodyObservation)
	x.httpStreamBytes -= len(body)
	direction.body = nil
	x.cleanupHTTPStream(key, stream)
	return manifest, err
}

func (x *objectExporter) releaseHTTPStreamBodies(stream *httpStreamState) {
	if stream == nil {
		return
	}
	for _, direction := range stream.directions {
		if direction != nil {
			x.httpStreamBytes -= len(direction.body)
			direction.body = nil
		}
	}
}

func (x *objectExporter) cleanupHTTPStream(key httpStreamKey, stream *httpStreamState) {
	if stream == nil {
		return
	}
	for _, direction := range stream.directions {
		if direction == nil || direction.header == nil || !direction.done {
			return
		}
	}
	delete(x.httpStream, key)
}

func containsPacketReference(values []pcaputil.PacketReference, want pcaputil.PacketReference) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsUint64(values []uint64, want uint64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func httpStreamBodyByteSource(body []byte, parents []uint64, refs []pcaputil.PacketReference) pcaputil.ByteSource {
	return assembledObjectByteSource(body, "reassembled", parents, refs)
}

func assembledObjectByteSource(body []byte, kind string, parents []uint64, refs []pcaputil.PacketReference) pcaputil.ByteSource {
	digest := sha256.Sum256(body)
	source := pcaputil.ByteSource{Kind: kind, SHA256: hex.EncodeToString(digest[:]), ParentPDUs: append([]uint64(nil), parents...), PacketRefs: append([]pcaputil.PacketReference(nil), refs...)}
	if len(refs) > 0 {
		completed := refs[len(refs)-1]
		source.CompletedIn = &completed
	}
	return source
}
