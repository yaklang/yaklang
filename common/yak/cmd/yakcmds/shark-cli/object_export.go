package sharkcli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type objectObservation struct {
	id, responseTo                                  uint64
	protocol, source, destination, uri, contentType string
	authorization, cookie                           any
	request                                         bool
	body                                            io.Reader
	declared                                        *int64
	refs                                            []pcaputil.PacketReference
	bytes                                           pcaputil.ByteSource
}

func observedObject(e *pcaputil.ProtocolEvent) *objectObservation {
	body, declared, response := e.HTTPResponseBody()
	if !response && e.Completeness != "message" && e.Completeness != "transaction" {
		return nil
	}
	snapshot := e.DisplayFieldValues()
	fields := snapshot.FieldMap()
	request := len(fields["http.request.method"]) > 0
	if !response && !request {
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
	return &objectObservation{id: e.ID, responseTo: e.ResponseTo, protocol: e.Protocol, source: e.Source, destination: e.Destination, request: request, body: body, declared: declared,
		uri: text("http.request.uri"), contentType: text("http.response.content_type"), authorization: literal("http.request.authorization"), cookie: literal("http.cookie"), refs: snapshot.Source.PacketRefs, bytes: snapshot.Source}
}

type objectExporter struct {
	directory    string
	requests     map[uint64]*objectObservation
	requestBytes int
}

func newObjectExporter(directory string) (*objectExporter, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create object directory: %w", err)
	}
	return &objectExporter{directory: directory, requests: make(map[uint64]*objectObservation)}, nil
}

func (x *objectExporter) observe(observations []*objectObservation) error {
	for _, observation := range observations {
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
		copy.refs = nil
		copy.bytes = pcaputil.ByteSource{}
		x.requests[observation.id] = &copy
		x.requestBytes += cost
	}
	return nil
}

func (x *objectExporter) forget(observation *objectObservation) {
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

type objectManifest struct {
	RecordType     string              `json:"record_type"`
	Physical       bool                `json:"physical"`
	PDU            uint64              `json:"pdu"`
	Protocol       string              `json:"protocol"`
	Source         string              `json:"source"`
	Destination    string              `json:"destination"`
	URI            any                 `json:"uri"`
	ContentType    string              `json:"content_type"`
	DeclaredLength *int64              `json:"declared_length"`
	ObservedLength int64               `json:"observed_length"`
	Complete       bool                `json:"complete"`
	LengthMismatch bool                `json:"length_mismatch,omitempty"`
	SHA256         string              `json:"sha256"`
	Path           string              `json:"path"`
	PacketRefs     []uint64            `json:"packet_refs"`
	ByteSource     pcaputil.ByteSource `json:"byte_source"`
	Authorization  any                 `json:"authorization"`
	Cookie         any                 `json:"cookie"`
}

func (x *objectExporter) export(o *objectObservation) (*objectManifest, error) {
	if o.body == nil {
		return nil, nil
	}
	var uri, authorization, cookie any
	uriText := ""
	if request := x.requests[o.responseTo]; request != nil {
		uriText = request.uri
		uri, authorization, cookie = request.uri, request.authorization, request.cookie
	}
	name := filepath.Join(x.directory, fmt.Sprintf("%d-%s", o.id, safeObjectName(uriText, o.contentType)))
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
	mismatch := o.declared != nil && *o.declared != written
	refs := make([]uint64, 0, len(o.refs))
	for _, ref := range o.refs {
		refs = append(refs, ref.Number)
	}
	return &objectManifest{RecordType: "object_export", PDU: o.id, Protocol: o.protocol, Source: o.source, Destination: o.destination, URI: uri, ContentType: o.contentType, DeclaredLength: o.declared, ObservedLength: written, Complete: !mismatch, LengthMismatch: mismatch, SHA256: hex.EncodeToString(hash.Sum(nil)), Path: name, PacketRefs: refs, ByteSource: o.bytes, Authorization: authorization, Cookie: cookie}, nil
}
