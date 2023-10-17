package lowhttp

import (
	"strings"
	"testing"
)

func TestLegacyRequestProxy(t *testing.T) {
	reqs, err := BuildLegacyProxyRequest([]byte(`HEAD / HTTP/1.1
Host: www.baidu.com`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(reqs), `HEAD http://www.baidu.com`) {
		t.Fatal("invalid legacy proxy request")
	}
}
