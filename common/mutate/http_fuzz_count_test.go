package mutate

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestCountHTTPRequestParamsMatchesMaterializedLists(t *testing.T) {
	jsonValue := `{"nested":{"key":"value"},"items":[1,2]}`
	base64JSON := base64.StdEncoding.EncodeToString([]byte(jsonValue))
	tests := []struct {
		name string
		raw  []byte
	}{
		{
			name: "query-and-cookie-variants",
			raw: []byte(fmt.Sprintf(
				"GET /path?plain=1&plain=2&json=%s&encoded=%s&=keyless&%%00=hidden HTTP/1.1\r\n"+
					"Host: example.test\r\n"+
					"Cookie: plain=value; json=%s; encoded=%s; PHPSESSID=ignored\r\n\r\n",
				jsonValue, base64JSON, jsonValue, base64JSON,
			)),
		},
		{
			name: "form-body",
			raw: []byte(
				"POST /form HTTP/1.1\r\nHost: example.test\r\n" +
					"Content-Type: application/x-www-form-urlencoded\r\n" +
					"Content-Length: 25\r\n\r\n" +
					"plain=1&json=%7B%22a%22%3A1%7D",
			),
		},
		{
			name: "json-body",
			raw: []byte(
				"POST /json HTTP/1.1\r\nHost: example.test\r\n" +
					"Content-Type: application/json\r\n" +
					"Content-Length: 46\r\n\r\n" +
					`{"top":{"nested":true},"items":[1,2,3]}`,
			),
		},
		{
			name: "xml-body",
			raw: []byte(
				"POST /xml HTTP/1.1\r\nHost: example.test\r\n" +
					"Content-Type: application/xml\r\n\r\n" +
					`<?xml version="1.0"?><root><!-- comment --><item id="1">one</item><item>two</item></root>`,
			),
		},
		{
			name: "soap-xml-body",
			raw: []byte(
				"POST /soap HTTP/1.1\r\nHost: example.test\r\n" +
					"Content-Type: application/xml\r\n\r\n" +
					`<soap:Envelope xmlns:soap="urn:test"><soap:Body><item>one</item></soap:Body></soap:Envelope>`,
			),
		},
		{
			name: "opaque-form-fallback",
			raw: []byte(
				"POST /opaque HTTP/1.1\r\nHost: example.test\r\n\r\n" +
					`plain=1&plain=2&json={"nested":[1,2]}`,
			),
		},
		{
			name: "empty-body",
			raw:  []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacyRequest, err := lowhttp.ParseBytesToHttpRequest(test.raw)
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := NewFuzzHTTPRequest(legacyRequest)
			if err != nil {
				t.Fatal(err)
			}
			wantGet := len(legacy.GetGetQueryParams())
			wantPost := len(legacy.GetPostCommonParams())
			wantCookie := len(legacy.GetCookieParams())

			directRequest, err := lowhttp.ParseBytesToHttpRequest(test.raw)
			if err != nil {
				t.Fatal(err)
			}
			gotGet, gotPost, gotCookie := CountHTTPRequestParams(directRequest)
			if gotGet != wantGet || gotPost != wantPost || gotCookie != wantCookie {
				t.Fatalf(
					"count mismatch: got get/post/cookie=%d/%d/%d, want %d/%d/%d",
					gotGet, gotPost, gotCookie, wantGet, wantPost, wantCookie,
				)
			}
		})
	}
}

func FuzzCountHTTPRequestParamsMatchesMaterializedLists(f *testing.F) {
	f.Add(
		`plain=1&json={"nested":[1,2]}&plain=2`,
		`{"top":{"nested":true},"items":[1,2,3]}`,
		`plain=value; encoded=eyJhIjoxfQ==; PHPSESSID=ignored`,
	)
	f.Add(
		`=keyless&%00=hidden&encoded=eyJhIjpbMSwyXX0=`,
		`<soap:Envelope xmlns:soap="urn:test"><soap:Body><item>one</item></soap:Body></soap:Envelope>`,
		`json={"a":{"b":true}}; malformed`,
	)
	f.Add("", `plain=1&plain=2&json=%7B%22a%22%3A1%7D`, "")

	newRequest := func(query, body, cookie string) *http.Request {
		header := make(http.Header)
		if cookie != "" {
			header.Set("Cookie", cookie)
		}
		return &http.Request{
			Method: "POST",
			URL:    &url.URL{Path: "/", RawQuery: query},
			Header: header,
			Body:   io.NopCloser(strings.NewReader(body)),
		}
	}

	f.Fuzz(func(t *testing.T, query, body, cookie string) {
		if len(query) > 4096 || len(body) > 4096 || len(cookie) > 4096 {
			t.Skip()
		}

		legacyRequest := newRequest(query, body, cookie)
		legacy := &FuzzHTTPRequest{_originRequestInstance: legacyRequest}
		wantGet := len(legacy.GetGetQueryParams())
		wantPost := len(legacy.GetPostCommonParams())
		wantCookie := len(legacy.GetCookieParams())

		gotGet, gotPost, gotCookie := CountHTTPRequestParams(newRequest(query, body, cookie))
		if gotGet != wantGet || gotPost != wantPost || gotCookie != wantCookie {
			t.Fatalf(
				"count mismatch: got get/post/cookie=%d/%d/%d, want %d/%d/%d",
				gotGet, gotPost, gotCookie, wantGet, wantPost, wantCookie,
			)
		}
	})
}

func BenchmarkCountHTTPRequestParamsParsedRequest(b *testing.B) {
	body := []byte(`{"top":{"nested":"value"},"items":[1,2,3,4],"plain":"text"}`)
	raw := []byte(fmt.Sprintf(
		"POST /path?plain=1&json=%%7B%%22a%%22%%3A1%%7D HTTP/1.1\r\n"+
			"Host: example.test\r\nContent-Type: application/json\r\n"+
			"Cookie: session=value; flags=%%7B%%22debug%%22%%3Atrue%%7D\r\n"+
			"Content-Length: %d\r\n\r\n%s",
		len(body), body,
	))

	b.Run("legacy-dump-and-reparse", func(b *testing.B) {
		req, err := lowhttp.ParseBytesToHttpRequest(raw)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fuzzReq, err := NewFuzzHTTPRequest(req)
			if err != nil {
				b.Fatal(err)
			}
			_, _, _ = len(fuzzReq.GetGetQueryParams()),
				len(fuzzReq.GetPostCommonParams()),
				len(fuzzReq.GetCookieParams())
		}
	})

	b.Run("parsed-list-materialization", func(b *testing.B) {
		req, err := lowhttp.ParseBytesToHttpRequest(raw)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fuzzReq := &FuzzHTTPRequest{_originRequestInstance: req}
			_, _, _ = len(fuzzReq.GetGetQueryParams()),
				len(fuzzReq.GetPostCommonParams()),
				len(fuzzReq.GetCookieParams())
		}
	})

	b.Run("count-only", func(b *testing.B) {
		req, err := lowhttp.ParseBytesToHttpRequest(raw)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, _ = CountHTTPRequestParams(req)
		}
	})
}
