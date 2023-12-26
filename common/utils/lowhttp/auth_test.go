package lowhttp

import (
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"testing"
)

func TestHTTPAuth(t *testing.T) {

	authHeader := []string{
		"WWW-Authenticate",
		"Www-Authenticate", // go fix
		"www-authenticate",
		"WWW-AUTHENTICATE",
	}
	count := 0
	username, passwd := "test", "test"
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		u, p, ok := request.BasicAuth()
		if ok && u == username && p == passwd {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header()[authHeader[count]] = []string{`Basic realm="restricted", charset="UTF-8"`}
		w.WriteHeader(http.StatusUnauthorized)
		count++
	})

	target := utils.HostPort(host, port)
	for i := 0; i < len(authHeader); i++ {
		rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: "+target+"\r\n\r\n")), WithUsername(username), WithPassword(passwd))
		if err != nil {
			t.Fatal(err)
		}
		if _, code, _ := GetHTTPPacketFirstLine(rsp.RawPacket); code != "200" {
			t.Fatalf("auth error want 200 get %v", code)
		}
	}
}
