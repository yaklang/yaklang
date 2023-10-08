package lowhttp

import (
	"testing"
)

func TestComputeDigestResponseFromRequest(t *testing.T) {
	req := BasicRequest()
	req = SetHTTPPacketUrl(req, "https://pie.dev/digest-auth/auth/admin/admin123/MD5")
	req = ReplaceHTTPPacketMethod(req, "GET")

	rsp, err := HTTP(WithPacketBytes(req), WithHttps(true), WithProxy("http://127.0.0.1:7890"))
	if err != nil {
		t.Fatal(err)
	}
	authorization := GetHTTPPacketHeader(rsp.RawPacket, "WWW-Authenticate")
	authHeader, err := GetDigestAuthorizationFromRequest(req, authorization, "admin", "admin123")
	if err != nil {
		t.Fatal(err)
	}
	req = ReplaceHTTPPacketHeader(req, "Authorization", authHeader)
	rsp, err = HTTP(WithPacketBytes(req), WithHttps(true), WithProxy("http://127.0.0.1:7890"))
	if err != nil {
		t.Fatal(err)
	}
	sc := GetStatusCodeFromResponse(rsp.RawPacket)
	if sc != 200 {
		t.Fatalf("want status code 200, but got %d", sc)
	}
}

func TestComputeDigestResponseFromRequestEx(t *testing.T) {
	req := BasicRequest()
	req = SetHTTPPacketUrl(req, "https://pie.dev/digest-auth/auth/admin/admin123/MD5")
	req = ReplaceHTTPPacketMethod(req, "GET")

	rsp, err := HTTP(WithPacketBytes(req), WithHttps(true), WithProxy("http://127.0.0.1:7890"))
	if err != nil {
		t.Fatal(err)
	}
	authorization := GetHTTPPacketHeader(rsp.RawPacket, "WWW-Authenticate")

	// wrong
	dr, ah, err := GetDigestAuthorizationFromRequestEx("GET", "https://pie.dev/digest-auth/auth/admin/admin123/MD5", "", authorization, "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req = ReplaceHTTPPacketHeader(req, "Authorization", ah.String())
	rsp, err = HTTP(WithPacketBytes(req), WithHttps(true), WithProxy("http://127.0.0.1:7890"))
	if err != nil {
		t.Fatal(err)
	}
	sc := GetStatusCodeFromResponse(rsp.RawPacket)
	if sc == 200 {
		t.Fatalf("want status code is not 200, got 200")
	}

	// success
	dr.UpdateRequestWithUsernameAndPassword("admin", "admin123")
	ah.RefreshAuthorizationWithoutConce(dr)
	req = ReplaceHTTPPacketHeader(req, "Authorization", ah.String())
	rsp, err = HTTP(WithPacketBytes(req), WithHttps(true), WithProxy("http://127.0.0.1:7890"))
	if err != nil {
		t.Fatal(err)
	}
	sc = GetStatusCodeFromResponse(rsp.RawPacket)
	if sc != 200 {
		t.Fatalf("want status code 200, but got %d", sc)
	}
}

func TestComputeDigestResponseFromRequestEx2(t *testing.T) {
	authorization := `Digest realm="IPCAM", nonce="ab513aea080440b021ec2015a2f3b959"`

	// wrong
	dr, ah, err := GetDigestAuthorizationFromRequestEx("DESCRIBE", "rtsp://172.27.252.174:8554/publisher", "", authorization, "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	dr.UpdateRequestWithUsernameAndPassword("admin", "admin")
	ah.Qop = ""
	ah.Cnonce = ""
	ah.URI = "rtsp://172.27.252.174:8554/publisher"
	ah.RefreshAuthorizationWithoutConce(dr)
	_ = dr
	t.Logf("response: %v", ah.String())
}
