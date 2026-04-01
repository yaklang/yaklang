package mimetype

import (
	"bytes"
	"mime"
	"strings"
)

// IsBinaryContentType reports whether the provided Content-Type should be
// treated as binary payload data.
func IsBinaryContentType(contentType string) bool {
	if contentType == "" {
		return false
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.ToLower(strings.Split(contentType, ";")[0]))
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))

	if mediaType == "" {
		return false
	}

	if strings.HasPrefix(mediaType, "text/") {
		return false
	}
	if strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml") {
		return false
	}

	switch mediaType {
	case "application/json",
		"application/xml",
		"text/xml",
		"application/javascript",
		"application/x-javascript",
		"application/ecmascript",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
		"application/x-ndjson",
		"application/graphql-response+json":
		return false
	}

	if strings.HasPrefix(mediaType, "image/") ||
		strings.HasPrefix(mediaType, "audio/") ||
		strings.HasPrefix(mediaType, "video/") ||
		strings.HasPrefix(mediaType, "font/") {
		return true
	}

	switch mediaType {
	case "application/octet-stream",
		"application/pdf",
		"application/zip",
		"application/gzip",
		"application/x-gzip",
		"application/x-rar-compressed",
		"application/vnd.rar",
		"application/x-7z-compressed",
		"application/x-tar",
		"application/x-bzip",
		"application/x-bzip2",
		"application/protobuf",
		"application/x-protobuf",
		"application/grpc",
		"application/wasm",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-powerpoint",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return true
	}

	return false
}

// ParseContentType builds a MIME object from an HTTP Content-Type string.
// It prefers a known MIME node from the detection tree and falls back to a
// lightweight MIME value when the declared type is not part of the tree.
func ParseContentType(contentType string) *MIME {
	if contentType == "" {
		return nil
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.ToLower(strings.Split(contentType, ";")[0]))
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		return nil
	}

	if mimeType := Lookup(mediaType); mimeType != nil {
		return mimeType
	}
	return &MIME{mime: mediaType}
}

// DetectResponse detects the MIME of an HTTP response packet.
// It prefers the response Content-Type header and falls back to body sniffing
// when the header is missing or unusable.
func DetectResponse(packet []byte) *MIME {
	if len(packet) == 0 {
		return Detect(nil)
	}

	contentType, body := splitHTTPResponseContentTypeAndBody(packet)
	if mimeType := ParseContentType(contentType); mimeType != nil {
		return mimeType
	}
	return Detect(body)
}

func splitHTTPResponseContentTypeAndBody(packet []byte) (string, []byte) {
	header, body, ok := splitHTTPPacket(packet)
	if !ok {
		return "", packet
	}

	lines := strings.Split(string(header), "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(lines[0])), "HTTP/") {
		return "", body
	}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "Content-Type") {
			return strings.TrimSpace(value), body
		}
	}
	return "", body
}

func splitHTTPPacket(packet []byte) ([]byte, []byte, bool) {
	if idx := bytes.Index(packet, []byte("\r\n\r\n")); idx >= 0 {
		return packet[:idx], packet[idx+4:], true
	}
	if idx := bytes.Index(packet, []byte("\n\n")); idx >= 0 {
		return packet[:idx], packet[idx+2:], true
	}
	return nil, nil, false
}
