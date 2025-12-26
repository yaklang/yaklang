package yakgrpc

import (
	"bytes"
	"testing"
)

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

	t.Run("hasHashedJSFilename", func(t *testing.T) {
		cases := []struct {
			name string
			want bool
		}{
			{"framework-5e497f6bfe92c60d1ebf.js", true},
			{"main.0b8e744a.js", true},
			{"index-7a0f5f5d7431abc5.js", true},
			{"0b8e744a-7a0f5f5d7431abc5.js", true},
			{"main.js", false},
			{"main.min.js", false},
			{"", false},
		}
		for _, c := range cases {
			if got := hasHashedJSFilename(c.name); got != c.want {
				t.Fatalf("hasHashedJSFilename(%q)=%v, want %v", c.name, got, c.want)
			}
		}
	})

	t.Run("isCacheControlStrong", func(t *testing.T) {
		cases := []struct {
			cc   string
			want bool
		}{
			{"public,max-age=31536000,immutable", true},
			{"public, max-age=86400", true},
			{"public, max-age=86399", false},
			{"no-store", false},
			{"", false},
			{"PUBLIC, MAX-AGE=31536000, IMMUTABLE", true},
		}
		for _, c := range cases {
			if got := isCacheControlStrong(c.cc); got != c.want {
				t.Fatalf("isCacheControlStrong(%q)=%v, want %v", c.cc, got, c.want)
			}
		}
	})

	t.Run("bundledJSSignatureScore", func(t *testing.T) {
		cases := []struct {
			body []byte
			want int
		}{
			{[]byte(`function(){return __webpack_require__(1)}`), 2},
			{[]byte(`self["webpackChunkapp"] = self["webpackChunkapp"] || [];`), 2},
			{[]byte(`regeneratorRuntime.mark(function(){});`), 1},
			{[]byte(`/*#__PURE__*/ foo();`), 1},
			{[]byte(`var _interopRequireDefault = function(){};`), 1},
			{[]byte(`Object.defineProperty(exports, "__esModule", { value: true });`), 1},
			{[]byte(`"use strict";\nconsole.log("x")`), 1},
			{[]byte(`console.log("x")`), 0},
			{nil, 0},
		}
		for _, c := range cases {
			if got := bundledJSSignatureScore(c.body); got != c.want {
				t.Fatalf("bundledJSSignatureScore(%q)=%v, want %v", string(c.body), got, c.want)
			}
		}
	})

	t.Run("shouldFilterBundledJavaScript", func(t *testing.T) {
		type tc struct {
			name          string
			urlPath       string
			contentType   string
			cacheControl  string
			contentLength int64
			body          []byte
			want          bool
		}

		signatureWithinLimitBody := bytes.Repeat([]byte("a"), 63*1024)
		signatureWithinLimitBody = append(signatureWithinLimitBody, []byte("__webpack_require__")...)
		signatureBeyondLimitBody := bytes.Repeat([]byte("a"), 70*1024)
		signatureBeyondLimitBody = append(signatureBeyondLimitBody, []byte("__webpack_require__")...)

		cases := []tc{
			{
				name:         "StrongPath_NextChunk",
				urlPath:      "/_next/static/chunks/0b8e744a-7a0f5f5d7431abc5.js",
				contentType:  "application/javascript; charset=utf-8",
				cacheControl: "public, max-age=31536000, immutable",
				want:         true,
			},
			{
				name:         "StrongPath_StaticJS",
				urlPath:      "/static/js/main.js",
				contentType:  "application/javascript",
				cacheControl: "no-store",
				want:         true,
			},
			{
				name:         "StrongPath_ChunkNamed",
				urlPath:      "/assets/app.chunk.js",
				contentType:  "application/javascript",
				cacheControl: "no-store",
				want:         true,
			},
			{
				name:         "WeakPath_HashFilename",
				urlPath:      "/assets/framework-5e497f6bfe92c60d1ebf.js",
				contentType:  "application/javascript",
				cacheControl: "no-store",
				want:         true,
			},
			{
				name:         "WeakPath_StrongCache",
				urlPath:      "/assets/custom.js",
				contentType:  "application/javascript",
				cacheControl: "public, max-age=31536000, immutable",
				want:         true,
			},
			{
				name:          "WeakPath_SignatureScore2",
				urlPath:       "/assets/vendors~chunk.js",
				contentType:   "application/javascript",
				cacheControl:  "",
				contentLength: 50 * 1024,
				body:          []byte("/* test */ function(){return __webpack_require__(1)}"),
				want:          true,
			},
			{
				name:          "WeakPath_SignatureScore1AndLarge",
				urlPath:       "/assets/custom.js",
				contentType:   "application/javascript",
				cacheControl:  "no-store",
				contentLength: 200 * 1024,
				body:          []byte(`"use strict";`),
				want:          true,
			},
			{
				name:          "WeakPath_NoSignals",
				urlPath:       "/static/custom.js",
				contentType:   "application/javascript",
				cacheControl:  "no-store",
				contentLength: 8 * 1024,
				body:          []byte("console.log('hello');"),
				want:          false,
			},
			{
				name:         "RootPath_HashAndStrongCache",
				urlPath:      "/framework-5e497f6bfe92c60d1ebf.js",
				contentType:  "application/javascript; charset=utf-8",
				cacheControl: "public, max-age=31536000, immutable",
				want:         true,
			},
			{
				name:          "RootPath_LargeCachedMinJS",
				urlPath:       "/babel-standalone@6.26.0/babel.min.js",
				contentType:   "text/javascript; charset=utf-8",
				cacheControl:  "public, max-age=31536000",
				contentLength: 791236,
				want:          true,
			},
			{
				name:          "RootPath_LargeCachedButNotMin",
				urlPath:       "/babel-standalone@6.26.0/babel.js",
				contentType:   "text/javascript; charset=utf-8",
				cacheControl:  "public, max-age=31536000",
				contentLength: 791236,
				want:          false,
			},
			{
				name:          "RootPath_HashWithoutStrongCache",
				urlPath:       "/framework-5e497f6bfe92c60d1ebf.js",
				contentType:   "application/javascript",
				cacheControl:  "no-store",
				contentLength: 129960,
				want:          false,
			},
			{
				name:          "NotJSContentType",
				urlPath:       "/assets/app.chunk.js",
				contentType:   "text/html; charset=utf-8",
				cacheControl:  "public, max-age=31536000, immutable",
				contentLength: 1000,
				want:          false,
			},
			{
				name:          "NonPath",
				urlPath:       "assets/app.chunk.js",
				contentType:   "application/javascript",
				cacheControl:  "public, max-age=31536000, immutable",
				contentLength: 1000,
				want:          false,
			},
			{
				name:          "SignatureWithinScanLimit",
				urlPath:       "/assets/vendors~chunk.js",
				contentType:   "application/javascript",
				cacheControl:  "",
				contentLength: 400 * 1024,
				body:          signatureWithinLimitBody,
				want:          true,
			},
			{
				name:          "SignatureBeyondScanLimit",
				urlPath:       "/assets/vendors~chunk.js",
				contentType:   "application/javascript",
				cacheControl:  "",
				contentLength: 400 * 1024,
				body:          signatureBeyondLimitBody,
				want:          false,
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got := shouldFilterBundledJavaScript(
					c.urlPath,
					c.contentType,
					c.cacheControl,
					c.contentLength,
					c.body,
				)
				if got != c.want {
					t.Fatalf("shouldFilterBundledJavaScript(url=%q, ct=%q, cc=%q, len=%d)=%v, want %v",
						c.urlPath, c.contentType, c.cacheControl, c.contentLength, got, c.want)
				}
			})
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
