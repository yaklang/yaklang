package yakhttp

import (
	"bytes"
	"testing"
)

func TestHttpRequestWithSession(t *testing.T) {
	// same session
	_, err := httpRequest("GET", "https://pie.dev/cookies/set/name1/value1", Session("test"))
	if err != nil {
		t.Fatal(err)
	}

	rsp, err := httpRequest("GET", "https://pie.dev/cookies", Session("test"))
	if err != nil {
		t.Fatal(err)
	}
	rspRaw, err := dump(rsp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rspRaw, []byte(`"name1": "value1"`)) {
		t.Fatalf("session failed, response: %s", rspRaw)
	}

	// not a same session
	_, err = httpRequest("GET", "https://pie.dev/cookies/set/name1/value1", Session("test1"))
	if err != nil {
		t.Fatal(err)
	}

	rsp, err = httpRequest("GET", "https://pie.dev/cookies", Session("test2"))
	if err != nil {
		t.Fatal(err)
	}
	rspRaw, err = dump(rsp)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rspRaw, []byte(`"name1": "value1"`)) {
		t.Fatalf("session failed, response: %s", rspRaw)
	}
}
