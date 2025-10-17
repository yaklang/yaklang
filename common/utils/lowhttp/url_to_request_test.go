package lowhttp

import (
	"bytes"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func CheckRequest(t *testing.T, raw []byte, wantReq string) {
	t.Helper()

	raw = FixHTTPRequest(raw)
	wantRaw := FixHTTPRequest([]byte(wantReq))

	reqIns, err := ParseBytesToHttpRequest(raw)
	require.NoError(t, err, "parse request error")
	wantReqIns, err := ParseBytesToHttpRequest(wantRaw)
	require.NoError(t, err, "parse want request error")

	// compare method
	if reqIns.Method != wantReqIns.Method {
		require.Equal(t, wantReqIns.Method, reqIns.Method, "method")
	}
	// compare url
	if reqIns.URL.String() != wantReqIns.URL.String() {
		require.Equal(t, wantReqIns.URL.String(), reqIns.URL.String(), "url")
	}
	// compare header
	if len(reqIns.Header) != len(wantReqIns.Header) {
		require.Len(t, reqIns.Header, len(wantReqIns.Header), "header len")
	} else {
		for k, v := range reqIns.Header {
			require.Greater(t, len(v), 0, "header %s is empty", k)
			require.Greater(t, len(wantReqIns.Header[k]), 0, "want header %s is empty", k)

			header, wantHeader := v[0], wantReqIns.Header[k][0]
			if k == "Cookie" {
				// sort header and wantHeader
				headers := lo.FilterMap(strings.Split(header, ";"), func(item string, index int) (string, bool) {
					trimed := strings.TrimSpace(item)
					return trimed, trimed != ""
				})
				wantHeaders := lo.FilterMap(strings.Split(wantHeader, ";"), func(item string, index int) (string, bool) {
					trimed := strings.TrimSpace(item)
					return trimed, trimed != ""
				})
				sort.Strings(headers)
				sort.Strings(wantHeaders)
				header = strings.Join(headers, "; ")
				wantHeader = strings.Join(wantHeaders, "; ")
			}
			require.Equalf(t, wantHeader, header, "Header %s", k)
		}
	}

	// compare body
	if reqIns.Body == nil && wantReqIns.Body != nil {
		t.Fatal("raw body is nil")
	}
	if reqIns.Body != nil && wantReqIns.Body == nil {
		t.Fatal("new body is nil")
	}
	if reqIns.Body != nil && wantReqIns.Body != nil {
		var buf1, buf2 bytes.Buffer
		_, _ = buf1.ReadFrom(reqIns.Body)
		_, _ = buf2.ReadFrom(wantReqIns.Body)
		require.Equal(t, buf2.String(), buf1.String(), "body")
	}
}

func TestUrlToGetRequestPacket(t *testing.T) {
	// keep header
	result := UrlToGetRequestPacket("https://example.com/asd", []byte(`GET /qwe HTTP/1.1
Host: example.com
AAA: BBB
Cookie: test=12;`), true)
	wantResult := `GET /asd HTTP/1.1
Host: example.com
AAA: BBB
Referer: https://example.com/qwe
`
	CheckRequest(t, result, wantResult)
}

func TestUrlToRequestPacketEx(t *testing.T) {
	t.Run("nil origin request", func(t *testing.T) {
		result, err := UrlToRequestPacketEx(http.MethodGet, "https://example.com/asd", nil, true, -1)
		require.NoError(t, err)

		wantResult := `GET /asd HTTP/1.1
Host: example.com

`
		CheckRequest(t, result, wantResult)
	})
	t.Run("referer", func(t *testing.T) {
		result, err := UrlToRequestPacketEx("", "https://example.com/qwe", []byte(`POST /asd HTTP/1.1
Host: example.com
AAA: BBB
Cookie: test=12;

aaa`), false, 302, nil)
		require.NoError(t, err)

		wantResult := `GET /qwe HTTP/1.1
Host: example.com
AAA: BBB
Referer: http://example.com/asd
`
		CheckRequest(t, result, wantResult)
	})
	t.Run("302", func(t *testing.T) {
		result, err := UrlToRequestPacketEx("", "https://example.com/qwe", []byte(`POST /asd HTTP/1.1
Host: example.com
AAA: BBB
Cookie: test=12;

aaa`), true, 302, nil)
		require.NoError(t, err)

		wantResult := `GET /qwe HTTP/1.1
Host: example.com
AAA: BBB
Referer: https://example.com/asd
`
		CheckRequest(t, result, wantResult)
	})
	t.Run("307", func(t *testing.T) {
		result, err := UrlToRequestPacketEx("", "https://example.com/qwe", []byte(`POST /asd HTTP/1.1
Host: example.com
AAA: BBB
Cookie: test=12;

aaa`), true, 307, nil)
		require.NoError(t, err)

		wantResult := `POST /qwe HTTP/1.1
Host: example.com
AAA: BBB
Referer: https://example.com/asd

aaa`
		CheckRequest(t, result, wantResult)
	})
}

func TestUrlToHTTPRequest(t *testing.T) {
	type args struct {
		text string
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "raw path",
			args: args{text: "http://127.0.0.1:1231/abcdef%2f?a=1&b=2%2f"},
			want: []byte("GET /abcdef%2f?a=1&b=2%2f HTTP/1.1\r\nHost: 127.0.0.1:1231\r\nAccept-Encoding: gzip, deflate, br\r\nAccept: */*\r\nAccept-Language: en-US;q=0.9,en;q=0.8\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36\r\nCache-Control: max-age=0\r\n\r\n"),
		},
		{
			name: "raw fragment",
			args: args{text: "http://127.0.0.1:1231/abcdef/?a=1&b=2%2f#123%3E"},
			want: []byte("GET /abcdef/?a=1&b=2%2f#123%3E HTTP/1.1\r\nHost: 127.0.0.1:1231\r\nAccept-Encoding: gzip, deflate, br\r\nAccept: */*\r\nAccept-Language: en-US;q=0.9,en;q=0.8\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36\r\nCache-Control: max-age=0\r\n\r\n"),
		},
		{
			name: "raw fragment 2",
			args: args{text: "http://127.0.0.1:1231/abcdef/?a=1&b=2%2f#123%3E#"},
			want: []byte("GET /abcdef/?a=1&b=2%2f#123%3E# HTTP/1.1\r\nHost: 127.0.0.1:1231\r\nAccept-Encoding: gzip, deflate, br\r\nAccept: */*\r\nAccept-Language: en-US;q=0.9,en;q=0.8\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36\r\nCache-Control: max-age=0\r\n\r\n"),
		},
		{
			name: "end fragment",
			args: args{text: "http://127.0.0.1:1231/#"},
			want: []byte("GET /# HTTP/1.1\r\nHost: 127.0.0.1:1231\r\nAccept-Encoding: gzip, deflate, br\r\nAccept: */*\r\nAccept-Language: en-US;q=0.9,en;q=0.8\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36\r\nCache-Control: max-age=0\r\n\r\n"),
		},
		{
			name: "test url",
			args: args{text: "http://127.0.0.1:1231"},
			want: []byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:1231\r\nAccept-Encoding: gzip, deflate, br\r\nAccept: */*\r\nAccept-Language: en-US;q=0.9,en;q=0.8\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36\r\nCache-Control: max-age=0\r\n\r\n"),
		},
		{
			name: "test url2",
			args: args{text: "http://127.0.0.1:1231/"},
			want: []byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:1231\r\nAccept-Encoding: gzip, deflate, br\r\nAccept: */*\r\nAccept-Language: en-US;q=0.9,en;q=0.8\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36\r\nCache-Control: max-age=0\r\n\r\n"),
		},
		{
			name: "test uri",
			args: args{text: "127.0.0.1:1231"},
			want: []byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:1231\r\nAccept-Encoding: gzip, deflate, br\r\nAccept: */*\r\nAccept-Language: en-US;q=0.9,en;q=0.8\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36\r\nCache-Control: max-age=0\r\n\r\n"),
		},
		{
			name: "test url escape",
			args: args{text: "http://127.0.0.1:1231/%C0%AE"},
			want: []byte("GET /%C0%AE HTTP/1.1\r\nHost: 127.0.0.1:1231\r\nAccept-Encoding: gzip, deflate, br\r\nAccept: */*\r\nAccept-Language: en-US;q=0.9,en;q=0.8\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36\r\nCache-Control: max-age=0\r\n\r\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UrlToHTTPRequest(tt.args.text)
			if err != nil {
				t.FailNow()
				return
			}

			assert.Equalf(t, string(tt.want), string(got), "UrlToHTTPRequest(%v)", tt.args.text)
		})
	}
}

func TestFixURLScheme(t *testing.T) {
	t.Run("no scheme 80 port", func(t *testing.T) {
		require.Equal(t, "http://example.com", FixURLScheme("example.com:80"))
	})
	t.Run("no scheme 443 port", func(t *testing.T) {
		require.Equal(t, "https://example.com", FixURLScheme("example.com:443"))
	})
	t.Run("no scheme not normal port", func(t *testing.T) {
		require.Equal(t, "http://example.com:11111", FixURLScheme("example.com:11111"))
	})
	t.Run("normal http", func(t *testing.T) {
		require.Equal(t, "http://example.com:80", FixURLScheme("http://example.com:80"))
	})
	t.Run("normal https", func(t *testing.T) {
		require.Equal(t, "http://example.com:8443", FixURLScheme("http://example.com:8443"))
	})
}
func TestFixHttpURL(t *testing.T) {
	for _, testcase := range []struct {
		url    string
		expect string
		name   string
	}{
		// fix schema
		{
			name:   "fix scheme by port 1",
			url:    "example.com:80",
			expect: "http://example.com",
		},
		{
			name:   "fix scheme by port 2",
			url:    "example.com:443",
			expect: "https://example.com",
		},
		{
			name:   "fix scheme by default1",
			url:    "example.com",
			expect: "http://example.com",
		},
		{
			name:   "fix scheme by default2",
			url:    "example.com:801",
			expect: "http://example.com:801",
		},
		// simplify the host
		{
			name:   "simplify the host 1",
			url:    "http://example.com:80",
			expect: "http://example.com",
		},
		{
			name:   "simplify the host 2",
			url:    "https://example.com:443",
			expect: "https://example.com",
		},
		// simplify the host negative test
		{
			name:   "simplify the host negative test 1",
			url:    "http://example.com:443",
			expect: "http://example.com:443",
		},
		{
			name:   "simplify the host negative test 2",
			url:    "https://example.com:80",
			expect: "https://example.com:80",
		},
		// check path
		{
			name:   "check path",
			url:    "https://example.com:80/abc",
			expect: "https://example.com:80/abc",
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			u, err := FixHttpURL(testcase.url)
			if err != nil {
				t.Fatal(err)
			}
			require.Equal(t, testcase.expect, u)
		})
	}
}
