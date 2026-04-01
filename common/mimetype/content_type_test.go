package mimetype

import "testing"

func TestIsBinaryContentType(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "empty", contentType: "", want: false},
		{name: "text plain", contentType: "text/plain; charset=utf-8", want: false},
		{name: "json", contentType: "application/json", want: false},
		{name: "json suffix", contentType: "application/problem+json", want: false},
		{name: "xml suffix", contentType: "application/soap+xml", want: false},
		{name: "multipart", contentType: "multipart/form-data; boundary=abc", want: false},
		{name: "image", contentType: "image/png", want: true},
		{name: "audio", contentType: "audio/mpeg", want: true},
		{name: "octet stream", contentType: "application/octet-stream", want: true},
		{name: "pdf", contentType: "application/pdf", want: true},
		{name: "invalid with semicolon fallback", contentType: "application/pdf; bad", want: true},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsBinaryContentType(tc.contentType); got != tc.want {
				t.Fatalf("IsBinaryContentType(%q) = %v, want %v", tc.contentType, got, tc.want)
			}
		})
	}
}

func TestParseContentType(t *testing.T) {
	t.Parallel()

	mimeType := ParseContentType("text/html; charset=utf-8")
	if mimeType == nil {
		t.Fatal("ParseContentType returned nil")
	}
	if got := mimeType.String(); got != "text/html" {
		t.Fatalf("ParseContentType returned %q, want %q", got, "text/html")
	}

	fallback := ParseContentType("application/graphql-response+json; charset=utf-8")
	if fallback == nil {
		t.Fatal("ParseContentType fallback returned nil")
	}
	if got := fallback.String(); got != "application/graphql-response+json" {
		t.Fatalf("ParseContentType fallback returned %q, want %q", got, "application/graphql-response+json")
	}
}

func TestDetectResponse(t *testing.T) {
	t.Parallel()

	t.Run("prefer header", func(t *testing.T) {
		t.Parallel()
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/pdf\r\nContent-Length: 5\r\n\r\nhello")
		mimeType := DetectResponse(packet)
		if mimeType == nil {
			t.Fatal("DetectResponse returned nil")
		}
		if got := mimeType.String(); got != "application/pdf" {
			t.Fatalf("DetectResponse returned %q, want %q", got, "application/pdf")
		}
		if !mimeType.IsBinary() {
			t.Fatal("DetectResponse should return a binary MIME for application/pdf")
		}
	})

	t.Run("fallback to body", func(t *testing.T) {
		t.Parallel()
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 12\r\n\r\nhello world!")
		mimeType := DetectResponse(packet)
		if mimeType == nil {
			t.Fatal("DetectResponse returned nil")
		}
		if mimeType.IsBinary() {
			t.Fatalf("DetectResponse fallback returned binary MIME %q for plain text body", mimeType.String())
		}
	})
}
