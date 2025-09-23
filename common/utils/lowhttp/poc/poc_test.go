package poc

import (
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestPocWithRandomJA3(t *testing.T) {
	token := utils.RandStringBytes(128)
	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: %d\r\n\r\n%s", len(token), token)))

	for i := 0; i < 16; i++ {
		rsp, _, err := DoGET("http://"+utils.HostPort(host, port), WithRandomJA3(true))
		require.NoError(t, err)
		require.Containsf(t, string(rsp.RawPacket), token, "invalid response")
	}
}

func TestPocRequestWithSession(t *testing.T) {
	token, token2, token3 := utils.RandStringBytes(10), utils.RandStringBytes(10), utils.RandStringBytes(10)
	cookieStr := fmt.Sprintf("%s=%s", token, token2)

	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nConnection: close\r\nSet-Cookie: %s\r\n\r\n", cookieStr)))

	// get cookie from server
	_, _, err := HTTP(fmt.Sprintf(`GET / HTTP/1.1
Host: %s
`, utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)

	// test HTTP / DO
	// if request has cookie
	_, req, err := HTTP(fmt.Sprintf(`GET / HTTP/1.1
Host: %s
`, utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)
	require.Contains(t, string(req), cookieStr)

	_, req2, err := Do(http.MethodGet, fmt.Sprintf("http://%s", utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)
	cookie, err := req2.Cookie(token)
	require.NoError(t, err)
	require.Equal(t, token2, cookie.Value)
}

func TestWithPostParams(t *testing.T) {
	tests := []struct {
		name                string
		input               any
		expectedContentType string
		expectedParams      []string
		description         string
	}{
		{
			name:                "map_input",
			input:               map[string]string{"username": "admin", "password": "123456", "token": "abc123"},
			expectedContentType: "application/x-www-form-urlencoded",
			expectedParams:      []string{"username=admin", "password=123456", "token=abc123"},
			description:         "map input should be converted to form data with correct content type",
		},
		{
			name:                "empty_value_map",
			input:               map[string]string{"username": ""},
			expectedContentType: "application/x-www-form-urlencoded",
			expectedParams:      []string{"username="},
			description:         "empty value map should set content type and empty parameter",
		},
		{
			name:                "mutli_value_map",
			input:               map[string][]string{"username": {"admin", "tom", "jerry"}},
			expectedContentType: "application/x-www-form-urlencoded",
			expectedParams:      []string{"username=admin", "&", "username=tom", "username=jerry"},
			description:         "empty value map should set content type and empty parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				contentType := request.Header.Get("Content-Type")
				body := ""
				if request.Body != nil {
					bodyBytes, _ := io.ReadAll(request.Body)
					body = string(bodyBytes)
				}

				response := fmt.Sprintf("Content-Type: %s\nBody: %s", contentType, body)
				writer.WriteHeader(200)
				writer.Write([]byte(response))
			})

			requestURL := fmt.Sprintf("http://%s", utils.HostPort(host, port))

			rsp, req, err := DoPOST(requestURL, WithPostParams(tt.input))
			require.NoError(t, err, tt.description)
			require.NotNil(t, rsp, "Response should not be nil")
			require.NotNil(t, req, "Request should not be nil")

			t.Logf("raw packet:%s", rsp.RawRequest)

			if tt.expectedContentType != "" {
				require.Equal(t, tt.expectedContentType, req.Header.Get("Content-Type"))
			}

			require.NoError(t, err, "Should be able to parse form data")

			if tt.expectedParams != nil {
				for _, param := range tt.expectedParams {
					require.Contains(t, string(rsp.RawRequest), param)
				}
			}

			t.Logf("✓ %s: %s", tt.name, tt.description)
		})
	}
}

func TestExtractPostParams(t *testing.T) {
	type args struct {
		raw []byte
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "form_urlencoded_should_decode_plus",
			args: args{
				raw: []byte("POST /login HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 17\r\n\r\na=1&b=hello+world"),
			},
			want: map[string]string{
				"a": "1",
				"b": "hello world",
			},
		},
		{
			name: "json_string_should_not_parse_as_query",
			args: args{
				raw: []byte("HTTP/1.1 403 Forbidden\r\nContent-Type: application/json\r\nContent-Length: 13\r\n\r\n\"aaaaa+bbbbb\""),
			},
			want:    map[string]string{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractPostParams(tt.args.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractPostParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractPostParams() got = %v, want %v", got, tt.want)
			}
		})
	}
}
