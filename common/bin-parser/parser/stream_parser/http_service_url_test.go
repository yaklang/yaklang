package stream_parser

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeHTTPServiceURL(t *testing.T) {
	for _, tc := range []struct{ input, scheme, authority, host, port, path, escapedPath, query string }{
		{"http://wpad/wpad.dat", "http", "wpad", "wpad", "", "/wpad.dat", "/wpad.dat", ""},
		{"https://example.test:8443/a%2Fb?q=%23", "https", "example.test:8443", "example.test", "8443", "/a/b", "/a%2Fb", "q=%23"},
		{"http://[2001:db8::1]:80/", "http", "[2001:db8::1]:80", "2001:db8::1", "80", "/", "/", ""},
		{"http://[fe80::1%25en0]/", "http", "[fe80::1%en0]", "fe80::1%en0", "", "/", "/", ""},
		{"HTTP://127.0.0.1:0", "http", "127.0.0.1:0", "127.0.0.1", "0", "", "", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := decodeHTTPServiceURL(tc.input)
			want := map[string]any{"scheme": tc.scheme, "authority": tc.authority, "hostname": tc.host, "port": tc.port, "path": tc.path, "escaped_path": tc.escapedPath, "raw_query": tc.query}
			want["canonical_ip"] = ""
			if addr, err := netip.ParseAddr(tc.host); err == nil {
				want["canonical_ip"] = addr.String()
			}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, %v; want %#v", got, err, want)
			}
		})
	}
	for _, input := range []string{"", "/wpad.dat", "//host/a", "ftp://host/a", "http:host", "http:///a", "http://:80/a", "http://user:pass@host/a", "http://host/#", "http://host/#f", "http://host:/", "http://host:65536/", "http://host:-1/", "http://host:abc/", "http://[hostname]/", "http://[127.0.0.1]/", "http://2001:db8::1/", "http://host\n/a", "http://host/%xy", "http://host/" + strings.Repeat("a", 8192)} {
		t.Run(input[:min(len(input), 80)], func(t *testing.T) {
			if got, err := decodeHTTPServiceURL(input); err == nil || got != nil {
				t.Fatalf("invalid or out-of-profile URL accepted: %#v, %v", got, err)
			}
		})
	}
}

func TestDecodeHTTPServiceURLWireEscaping(t *testing.T) {
	for _, input := range []string{"http://host/a b", "http://host/a\\b", "http://host/?q=%xy", "http://host/?q=a b", "http://host/<x>", "http://host/é", "http://host/?q=[x]", "http://host/[x]", "http://host/?q=%"} {
		_, err := decodeHTTPServiceURL(input)
		if err == nil {
			t.Errorf("non-URI wire text accepted: %q", input)
		}
	}
	for _, input := range []string{"http://host/a%20b", "http://host/%C3%A9?q=%23", "http://host/?q=%5Bx%5D", "http://[2001:db8::1]/a"} {
		if _, err := decodeHTTPServiceURL(input); err != nil {
			t.Errorf("valid encoded URI %q: %v", input, err)
		}
	}
}
