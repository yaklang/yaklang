package utils

import "bytes"

var httpRequestMethodPrefixes = [][]byte{
	[]byte("GET "),
	[]byte("POST "),
	[]byte("PUT "),
	[]byte("DELETE "),
	[]byte("HEAD "),
	[]byte("OPTIONS "),
	[]byte("PATCH "),
	[]byte("CONNECT "),
	[]byte("TRACE "),
}

// IsHTTPRequestPrefix reports whether data starts with a known HTTP request
// method. It is intentionally strict: arbitrary binary protocols must not be
// handed to the permissive HTTP request parser.
func IsHTTPRequestPrefix(data []byte) bool {
	for _, prefix := range httpRequestMethodPrefixes {
		if bytes.HasPrefix(data, prefix) {
			return true
		}
	}
	return false
}

// CouldBeHTTPRequestPrefix reports whether more bytes could turn data into a
// recognized HTTP request prefix. It lets protocol sniffers reject binary
// traffic early instead of blocking while waiting for an HTTP-sized prefix.
func CouldBeHTTPRequestPrefix(data []byte) bool {
	for _, prefix := range httpRequestMethodPrefixes {
		if bytes.HasPrefix(prefix, data) || bytes.HasPrefix(data, prefix) {
			return true
		}
	}
	return false
}
