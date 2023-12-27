package lowhttp

import (
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"testing"
)

type authCase struct {
	key        string
	value      string
	expectCode string
}

func TestHTTPAuth(t *testing.T) {
	basicAuth := `Basic realm="restricted", charset="UTF-8"`
	authCases := []authCase{
		{
			key:        "WWW-Authenticate",
			value:      basicAuth,
			expectCode: "200",
		},
		{
			key:        "Www-Authenticate",
			value:      basicAuth,
			expectCode: "200",
		},
		{
			key:        "www-authenticate",
			value:      basicAuth,
			expectCode: "200",
		},
		{
			key:        "WWW-AUTHENTICATE",
			value:      basicAuth,
			expectCode: "200",
		},
		{
			key:        "WWW-AUTHENTICATE",
			value:      "",
			expectCode: "401",
		},
	}

	count := 0
	username, passwd := "test", "test"
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		u, p, ok := request.BasicAuth()
		if ok && u == username && p == passwd {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header()[authCases[count].key] = []string{authCases[count].value}
		w.WriteHeader(http.StatusUnauthorized)
		count++
	})

	target := utils.HostPort(host, port)
	for i := 0; i < len(authCases); i++ {
		rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: "+target+"\r\n\r\n")), WithUsername(username), WithPassword(passwd))
		if err != nil {
			t.Fatal(err)
		}
		if _, code, _ := GetHTTPPacketFirstLine(rsp.RawPacket); code != authCases[i].expectCode {
			t.Fatalf("auth error want %v get %v", authCases[i].expectCode, code)
		}
	}
}
