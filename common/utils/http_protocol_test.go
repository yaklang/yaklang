package utils

import "testing"

func TestHTTPRequestPrefixDetection(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		isHTTP    bool
		couldHTTP bool
	}{
		{name: "GET", data: []byte("GET / HTTP/1.1\r\n"), isHTTP: true, couldHTTP: true},
		{name: "partial GET", data: []byte("GE"), isHTTP: false, couldHTTP: true},
		{name: "QQ-like binary", data: []byte{0x00, 0x00, 0x00, 0x20}, isHTTP: false, couldHTTP: false},
		{name: "FTP command", data: []byte("USER anonymous\r\n"), isHTTP: false, couldHTTP: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsHTTPRequestPrefix(test.data); got != test.isHTTP {
				t.Fatalf("IsHTTPRequestPrefix() = %v, want %v", got, test.isHTTP)
			}
			if got := CouldBeHTTPRequestPrefix(test.data); got != test.couldHTTP {
				t.Fatalf("CouldBeHTTPRequestPrefix() = %v, want %v", got, test.couldHTTP)
			}
		})
	}
}
