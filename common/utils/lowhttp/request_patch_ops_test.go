package lowhttp

import (
	"strings"
	"testing"
)

func TestPatchHTTPPacketJSONField_PreserveExistingValueType(t *testing.T) {
	packet := []byte("POST /v1/checkout HTTP/1.1\r\nHost: pay.store.com\r\nContent-Type: application/json\r\n\r\n{\"order_id\":\"9981\",\"amount\":2000.00,\"currency\":\"CNY\",\"user_id\":\"888\"}")

	patched, err := PatchHTTPPacketJSONField(packet, "replace", "amount", "0.01")
	if err != nil {
		t.Fatalf("replace amount failed: %v", err)
	}
	if !strings.Contains(string(patched), `"amount":0.01`) {
		t.Fatalf("expected amount to remain numeric, got:\n%s", string(patched))
	}

	patched, err = PatchHTTPPacketJSONField(patched, "replace", "user_id", "1")
	if err != nil {
		t.Fatalf("replace user_id failed: %v", err)
	}
	if !strings.Contains(string(patched), `"user_id":"1"`) {
		t.Fatalf("expected user_id to remain string, got:\n%s", string(patched))
	}
}

func TestRewriteHTTPPacketBearerJWTClaims(t *testing.T) {
	packet := []byte("GET /admin HTTP/1.1\r\nHost: example.com\r\nAuthorization: Bearer eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJyb2xlIjoidXNlciIsInVpZCI6MX0\r\n\r\n")

	patched, err := RewriteHTTPPacketBearerJWTClaims(packet, `{"role":"admin"}`)
	if err != nil {
		t.Fatalf("rewrite bearer jwt claims failed: %v", err)
	}
	patchedText := string(patched)
	if !strings.Contains(patchedText, "Authorization: Bearer ") {
		t.Fatalf("expected bearer authorization header, got:\n%s", patchedText)
	}
	if !strings.Contains(patchedText, "eyJyb2xlIjoiYWRtaW4iLCJ1aWQiOjF9") {
		t.Fatalf("expected rewritten payload segment, got:\n%s", patchedText)
	}
}

func TestTransformHTTPPacketBodyFormat_JSONToXML(t *testing.T) {
	packet := []byte("POST /data HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\n\r\n{\"user\":\"guest\",\"action\":\"ping\",\"tags\":[\"test\",\"dev\"]}")

	patched, err := TransformHTTPPacketBodyFormat(packet, "xml", "root")
	if err != nil {
		t.Fatalf("transform json to xml failed: %v", err)
	}
	patchedText := string(patched)
	if !strings.Contains(patchedText, "Content-Type: application/xml") {
		t.Fatalf("expected xml content type, got:\n%s", patchedText)
	}
	if !strings.Contains(patchedText, "<root>") || !strings.Contains(patchedText, "<tags><item>test</item><item>dev</item></tags>") {
		t.Fatalf("expected converted xml body, got:\n%s", patchedText)
	}
}
