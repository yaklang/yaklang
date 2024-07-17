package lowhttp

import (
	"bytes"
	"net/http"
	"net/http/cookiejar"
	"sort"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
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
		result, err := UrlToRequestPacketEx(http.MethodGet, "https://example.com/asd", nil, true, -1, nil)
		require.NoError(t, err)

		wantResult := string(FixHTTPRequest([]byte(`GET /asd HTTP/1.1
Host: example.com

`)))
		CheckRequest(t, result, string(wantResult))
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

	t.Run("jar", func(t *testing.T) {
		jar, err := cookiejar.New(nil)
		require.NoError(t, err)

		urlIns := utils.ParseStringToUrl("https://example.com")
		jar.SetCookies(urlIns, []*http.Cookie{
			{
				Name:  "test",
				Value: "12",
			},
		})

		result, err := UrlToRequestPacketEx(http.MethodPost, "https://example.com/qwe", []byte(`POST /asd HTTP/1.1
Host: example.com
AAA: BBB
Content-Length: 4

ab
`), true, 307, jar)
		require.NoError(t, err)

		wantResult := `POST /qwe HTTP/1.1
Host: example.com
Cookie: test=12
AAA: BBB
Content-Length: 4
Referer: https://example.com/asd

ab
`
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
			want: []byte("GET /abcdef%2f?a=1&b=2%2f HTTP/1.1\r\nHost: 127.0.0.1:1231\r\n\r\n"),
		},
		{
			name: "raw fragment",
			args: args{text: "http://127.0.0.1:1231/abcdef/?a=1&b=2%2f#123%3E"},
			want: []byte("GET /abcdef/?a=1&b=2%2f#123%3E HTTP/1.1\r\nHost: 127.0.0.1:1231\r\n\r\n"),
		},
		{
			name: "raw fragment 2",
			args: args{text: "http://127.0.0.1:1231/abcdef/?a=1&b=2%2f#123%3E#"},
			want: []byte("GET /abcdef/?a=1&b=2%2f#123%3E# HTTP/1.1\r\nHost: 127.0.0.1:1231\r\n\r\n"),
		},
		{
			name: "end fragment",
			args: args{text: "http://127.0.0.1:1231/#"},
			want: []byte("GET /# HTTP/1.1\r\nHost: 127.0.0.1:1231\r\n\r\n"),
		},
		{
			name: "test url",
			args: args{text: "http://127.0.0.1:1231"},
			want: []byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:1231\r\n\r\n"),
		},
		{
			name: "test url2",
			args: args{text: "http://127.0.0.1:1231/"},
			want: []byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:1231\r\n\r\n"),
		},
		{
			name: "test uri",
			args: args{text: "127.0.0.1:1231"},
			want: []byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:1231\r\n\r\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UrlToHTTPRequest(tt.args.text)
			if err != nil {
				t.FailNow()
				return
			}
			assert.Equalf(t, tt.want, got, "UrlToHTTPRequest(%v)", tt.args.text)
		})
	}
}

func TestFixURL(t *testing.T) {
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
