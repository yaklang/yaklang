package lowhttp

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
	"testing"
)

func TestFixMultipartBody(t *testing.T) {
	_, body := FixMultipartBody([]byte(`------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo(); ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7--
`))
	println(string(codec.EncodeBase64(body)))
	spew.Dump(body)
	if !strings.Contains(string(body), "phpinfo();") {
		panic("FAILED")
	}
}

func TestFixMultipartBody_LFInBody(t *testing.T) {
	_, body := FixMultipartBody([]byte(`------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();` + "\n" + ` ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7--
`))
	println(string(codec.EncodeBase64(body)))
	spew.Dump(body)
	println(string(body))
	if strings.Contains(string(body), `phpinfo();`+CRLF) {
		panic("CRLF in Body")
	}
	if !strings.Contains(string(body), "phpinfo();") {
		panic("FAILED")
	}
}

func TestFixMultipartBody_LFInBody3(t *testing.T) {
	_, body := FixMultipartBody([]byte(`------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();` + "\n" + ` ?>
------------Ef1KM7GI3Ef1ei4Ij5ae
`))
	println(string(codec.EncodeBase64(body)))
	spew.Dump(body)
	println(string(body))
	if strings.Contains(string(body), `phpinfo();`+CRLF) {
		panic("CRLF in Body")
	}
	if !strings.Contains(string(body), "phpinfo();") {
		panic("FAILED")
	}
	// VerifyMultipart(b, body)
}

func TestFixMultipartBody_LFInBody2(t *testing.T) {
	_, body := FixMultipartBody([]byte(`------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();` + "\n" + ` ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7-----
`))
	println(string(codec.EncodeBase64(body)))
	spew.Dump(body)
	println(string(body))
	if strings.Contains(string(body), `phpinfo();`+CRLF) {
		panic("CRLF in Body")
	}
	if !strings.Contains(string(body), "phpinfo();") {
		panic("FAILED")
	}
	// VerifyMultipart(b, body)
}

func TestFixMultipartBody_LFInBody4(t *testing.T) {
	_, body := FixMultipartBody([]byte(`------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();` + "\n" + ` ?>
-------
`))
	println(string(codec.EncodeBase64(body)))
	spew.Dump(body)
	println(string(body))
	if strings.Contains(string(body), `phpinfo();`+CRLF) {
		panic("CRLF in Body")
	}

	if !strings.Contains(string(body), "phpinfo();") {
		panic("FAILED")
	}
	// VerifyMultipart(b, body)
}

func TestFixMultipartBody_LFInBody5(t *testing.T) {
	_, body := FixMultipartBody([]byte(`------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();` + "\n" + ` ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---



`))
	println(string(codec.EncodeBase64(body)))
	spew.Dump(body)
	println(string(body))
	if strings.Contains(string(body), `phpinfo();`+CRLF) {
		panic("CRLF in Body")
	}
	if !strings.Contains(string(body), "phpinfo();") {
		panic("FAILED")
	}

	if !strings.HasSuffix(string(body), ` ?>`+CRLF+`------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---`+"--"+CRLF) {
		panic("Identify end failed")
	}
	// VerifyMultipart(b, body)
}

func TestFixMultipartBody_LFInBody_Boundary(t *testing.T) {
	_, body := FixMultipartBody([]byte(`--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"
--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"


--------------------------123--`))
	println(string(codec.EncodeBase64(body)))
	spew.Dump(body)
	println(string(body))
	if !strings.Contains(string(body), `Content-Disposition: form-data; name="{\"key\": \"value\"}"`+CRLF+"---------") {
		panic("CRLF in Body")
	}
	// VerifyMultipart(b, body)
}

func TestFixMultipartBody_LFInBody_Boundary2(t *testing.T) {
	_, body := FixMultipartBody([]byte(`--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"

--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"


--------------------------123--`))
	println(string(codec.EncodeBase64(body)))
	spew.Dump(body)
	println(string(body))
	if !strings.Contains(string(body), `Content-Disposition: form-data; name="{\"key\": \"value\"}"`+CRLF+CRLF+"---------") {
		panic("CRLF in Body")
	}

	if !strings.Contains(string(body), `Content-Disposition: form-data; name="{\"key\": \"value\"}"`+CRLF+CRLF+CRLF+"---------") {
		panic("CRLF in Body")
	}
	// VerifyMultipart(b, body)
}

func TestFixMultipartPacket(t *testing.T) {
	// LS0tLS0tLS0tLS0tRWYxS003R0kzRWYxZWk0SWo1YWUwS003Y0gyS003DQpDb250ZW50LURpc3Bvc2l0aW9uOiBmb3JtLWRhdGE7IG5hbWU9ImZpbGUiOyBmaWxlbmFtZT0iYS5waHAiDQpDb250ZW50LVR5cGU6IGltYWdlL3BuZw0KDQo8P3BocCBwaHBpbmZvKCk7ID8+DQotLS0tLS0tLS0tLS1FZjFLTTdHSTNFZjFlaTRJajVhZTBLTTdjSDJLTTctLQ0K
	flag := `LS0tLS0tLS0tLS0tRWYxS003R0kzRWYxZWk0SWo1YWUwS003Y0gyS003DQpDb250ZW50LURpc3Bvc2l0aW9uOiBmb3JtLWRhdGE7IG5hbWU9ImZpbGUiOyBmaWxlbmFtZT0iYS5waHAiDQpDb250ZW50LVR5cGU6IGltYWdlL3BuZw0KDQo8P3BocCBwaHBpbmZvKCk7ID8+DQotLS0tLS0tLS0tLS1FZjFLTTdHSTNFZjFlaTRJajVhZTBLTTdjSDJLTTctLQ0K`
	raw, _ := codec.DecodeBase64(`LS0tLS0tLS0tLS0tRWYxS003R0kzRWYxZWk0SWo1YWUwS003Y0gyS003DQpDb250ZW50LURpc3Bvc2l0aW9uOiBmb3JtLWRhdGE7IG5hbWU9ImZpbGUiOyBmaWxlbmFtZT0iYS5waHAiDQpDb250ZW50LVR5cGU6IGltYWdlL3BuZw0KDQo8P3BocCBwaHBpbmZvKCk7ID8+DQotLS0tLS0tLS0tLS1FZjFLTTdHSTNFZjFlaTRJajVhZTBLTTdjSDJLTTctLQ0K`)
	_, raw = FixMultipartBody(raw)
	if flag != codec.EncodeBase64(raw) {
		panic(1)
	}
}
