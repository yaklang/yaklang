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
			{"/assets/vendors~chunk.js", false},
			{"/assets/chunk-12345.js", false},
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

	t.Run("shouldFilterBundledJavaScript", func(t *testing.T) {
		// next.js strong path
		if !shouldFilterBundledJavaScript(
			"/_next/static/chunks/0b8e744a-7a0f5f5d7431abc5.js",
			"application/javascript; charset=utf-8",
			"public, max-age=31536000, immutable",
			1024,
			nil,
		) {
			t.Fatal("expected next.js chunk to be filtered")
		}

		// assets 目录：通过 body signature 识别 webpack 引导
		if !shouldFilterBundledJavaScript(
			"/assets/vendors~chunk.js",
			"application/javascript",
			"",
			50*1024,
			[]byte("/* test */ function(){return __webpack_require__(1)}"),
		) {
			t.Fatal("expected bundled js signature to be filtered")
		}

		// static 目录：无 hash/无强缓存/无 signature，不过滤（可能是重要手写 JS）
		if shouldFilterBundledJavaScript(
			"/static/custom.js",
			"application/javascript",
			"no-store",
			8*1024,
			[]byte("console.log('hello');"),
		) {
			t.Fatal("expected custom js not to be filtered")
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
