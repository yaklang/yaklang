package pcaputil

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ippKey struct {
	dir int
	id  uint32
}
type binIPP struct{ pending map[ippKey]uint16 }

func ippMedia(s string) bool {
	return strings.EqualFold(strings.TrimSpace(strings.SplitN(s, ";", 2)[0]), "application/ipp")
}
func ippFields(w []byte, max, depthLimit int) (map[string]any, error) {
	if len(w) < 9 {
		return nil, fmt.Errorf("ipp: truncated header")
	}
	if w[0] != 1 && w[0] != 2 {
		return nil, protocolError(ErrUnsupportedVersion, "IPP version")
	}
	id := binary.BigEndian.Uint32(w[4:8])
	if id == 0 {
		return nil, fmt.Errorf("ipp: zero request ID")
	}
	out := map[string]any{"Version Major": w[0], "Version Minor": w[1], "Operation/Status": binary.BigEndian.Uint16(w[2:]), "Request ID": id}
	attrs := []any{}
	at, depth := 8, 0
	var group byte
	name := ""
	for at < len(w) {
		tag := w[at]
		at++
		if tag == 3 {
			if depth != 0 {
				return nil, fmt.Errorf("ipp: unclosed collection")
			}
			out["Attributes"] = attrs
			out["Document Bytes"] = len(w) - at
			return out, nil
		}
		if tag < 0x10 {
			if depth != 0 {
				return nil, fmt.Errorf("ipp: group inside collection")
			}
			switch tag {
			case 1, 2, 4, 5, 6, 7, 9, 10:
				group = tag
				name = ""
				continue
			default:
				return nil, fmt.Errorf("ipp: invalid group delimiter")
			}
		}
		if group == 0 || at+2 > len(w) {
			return nil, fmt.Errorf("ipp: attribute without group/header")
		}
		if len(attrs) >= max {
			return nil, protocolError(ErrResourceExceeded, "IPP attribute budget")
		}
		n := int(binary.BigEndian.Uint16(w[at:]))
		at += 2
		if at+n+2 > len(w) {
			return nil, fmt.Errorf("ipp: attribute name length")
		}
		if n > 0 {
			name = string(w[at : at+n])
		}
		at += n
		ln := int(binary.BigEndian.Uint16(w[at:]))
		at += 2
		if at+ln > len(w) {
			return nil, fmt.Errorf("ipp: value length")
		}
		v := w[at : at+ln]
		at += ln
		if name == "" && depth == 0 {
			return nil, fmt.Errorf("ipp: additional value lacks prior name")
		}
		a := map[string]any{"Group": group, "Tag": tag, "Name": name, "Depth": depth}
		switch tag {
		case 0x21, 0x23:
			if ln != 4 {
				return nil, fmt.Errorf("ipp: integer/enum length")
			}
			a["Value"] = int32(binary.BigEndian.Uint32(v))
		case 0x22:
			if ln != 1 || v[0] > 1 {
				return nil, fmt.Errorf("ipp: boolean length/value")
			}
			a["Value"] = v[0] == 1
		case 0x32:
			if ln != 9 {
				return nil, fmt.Errorf("ipp: resolution length")
			}
			a["Value"] = map[string]any{"X": binary.BigEndian.Uint32(v), "Y": binary.BigEndian.Uint32(v[4:]), "Units": v[8]}
		case 0x33:
			if ln != 8 {
				return nil, fmt.Errorf("ipp: range length")
			}
			a["Value"] = map[string]any{"Lower": int32(binary.BigEndian.Uint32(v)), "Upper": int32(binary.BigEndian.Uint32(v[4:]))}
		case 0x34:
			if ln != 0 || depth >= depthLimit {
				return nil, protocolError(ErrResourceExceeded, "IPP collection depth/length")
			}
			depth++
		case 0x37:
			if ln != 0 || n != 0 || depth == 0 {
				return nil, fmt.Errorf("ipp: invalid collection end")
			}
			depth--
		case 0x31:
			if ln != 11 {
				return nil, fmt.Errorf("ipp: date-time length")
			}
			a["Value"] = append([]byte(nil), v...)
		case 0x41, 0x42, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49, 0x4a:
			a["Value"] = string(v)
		default:
			a["Value"] = append([]byte(nil), v...)
		}
		attrs = append(attrs, a)
	}
	return nil, fmt.Errorf("ipp: missing end-of-attributes")
}
func (f *binFlow) consumeIPPHTTP(dir int, raw []byte) (map[string]any, error) {
	reader := bufio.NewReader(bytes.NewReader(raw))
	var body io.ReadCloser
	var media string
	response := bytes.HasPrefix(raw, []byte("HTTP/"))
	status := 0
	if response {
		r, err := http.ReadResponse(reader, &http.Request{Method: "POST"})
		if err != nil {
			return nil, err
		}
		body, media, status = r.Body, r.Header.Get("Content-Type"), r.StatusCode
	} else {
		r, err := http.ReadRequest(reader)
		if err != nil {
			return nil, err
		}
		body, media = r.Body, r.Header.Get("Content-Type")
		if r.Method != "POST" {
			body.Close()
			return nil, fmt.Errorf("ipp: request must use POST")
		}
	}
	defer body.Close()
	if !ippMedia(media) {
		return nil, protocolError(ErrContextRequired, "HTTP response did not carry an IPP result")
	}
	w, err := io.ReadAll(io.LimitReader(body, int64(f.a.config.MaxMessageBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(w) > f.a.config.MaxMessageBytes {
		return nil, protocolError(ErrResourceExceeded, "IPP body")
	}
	out, err := ippFields(w, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth)
	if err != nil {
		return nil, err
	}
	if f.ipp == nil {
		f.ipp = &binIPP{pending: map[ippKey]uint16{}}
	}
	s := f.ipp
	if err = f.reserveSession(256 + int64(len(s.pending)+1)*64); err != nil {
		return nil, err
	}
	id := out["Request ID"].(uint32)
	op := out["Operation/Status"].(uint16)
	key := ippKey{dir, id}
	out["IPP"] = true
	if response {
		key.dir = 1 - dir
		req, ok := s.pending[key]
		out["Matched"] = ok
		out["HTTP Status"] = status
		out["Packet Name"] = "Response"
		if ok {
			out["In Reply To"] = req
			delete(s.pending, key)
		}
	} else {
		if _, ok := s.pending[key]; ok {
			return nil, protocolError(ErrDesynchronized, "IPP request ID reused")
		}
		if len(s.pending) >= f.a.budget.MaxCollectionElements {
			return nil, protocolError(ErrResourceExceeded, "IPP pending requests")
		}
		s.pending[key] = op
		out["Packet Name"] = "Request"
	}
	return out, nil
}
