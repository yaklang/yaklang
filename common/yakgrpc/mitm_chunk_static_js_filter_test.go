package yakgrpc

import "testing"

func TestChunkStaticJSFilterHelpers(t *testing.T) {
	t.Run("isChunkStaticJSRequest", func(t *testing.T) {
		cases := []struct {
			path string
			want bool
		}{
			{"/_next/static/chunks/0b8e744a-7a0f5f5d7431abc5.js", true},
			{"/static/js/main.js", true},
			{"/static/js/main.js.map", false},
			{"/assets/app.chunk.js", true},
			{"/assets/vendors~chunk.js", true},
			{"/assets/chunk-12345.js", true},
			{"/assets/chunk-12345.css", false},
			{"/api/v1/chunk", false},
			{"/api/v1/chunk.js", false},
			{"api/v1/chunk.js", false},
		}
		for _, c := range cases {
			if got := isChunkStaticJSRequest(c.path); got != c.want {
				t.Fatalf("isChunkStaticJSRequest(%q)=%v, want %v", c.path, got, c.want)
			}
		}
	})

	t.Run("isJavaScriptMIME", func(t *testing.T) {
		cases := []struct {
			ct   string
			want bool
		}{
			{"application/javascript", true},
			{"application/javascript; charset=utf-8", true},
			{"text/javascript", true},
			{"application/x-javascript", true},
			{"application/ecmascript", true},
			{"text/ecmascript", true},
			{"application/ld+json", false},
			{"text/html", false},
			{"", false},
		}
		for _, c := range cases {
			if got := isJavaScriptMIME(c.ct); got != c.want {
				t.Fatalf("isJavaScriptMIME(%q)=%v, want %v", c.ct, got, c.want)
			}
		}
	})
}
