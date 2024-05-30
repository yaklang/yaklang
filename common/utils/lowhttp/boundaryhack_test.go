package lowhttp

import (
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func TestFixMultipartBody2(t *testing.T) {
	_, body := FixMultipartBody([]byte(`--a
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo(); ?>` + "\n" + `--a--
`))
	println(string(codec.EncodeBase64(body)))
	println("--------------------")
	spew.Dump(body)
	if !strings.Contains(string(body), "phpinfo(); ?>\r\n--a--") {
		panic("FAILED")
	}
}

func TestFixMultipartBody3(t *testing.T) {
	_, body := FixMultipartBody([]byte(`--a
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo(); ?>` + "\n" + `--aa--
--a--
`))
	println(string(codec.EncodeBase64(body)))
	println("--------------------")
	spew.Dump(body)
	if !strings.Contains(string(body), "phpinfo(); ?>\n--aa--\r\n--a--") {
		panic("FAILED")
	}
}

func TestFixMultipartBody(t *testing.T) {
	_, body := FixMultipartBody([]byte(`------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo(); ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7--
`))
	println(string(codec.EncodeBase64(body)))
	println("--------------------")
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
	if !strings.Contains(string(body), "phpinfo();\n") {
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
	println("-------------------------")
	spew.Dump(body)
	println("-------------------------")
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
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---` + "\n\n\n\r\n"))
	println(string(codec.EncodeBase64(body)))
	spew.Dump(body)
	println(string(body))
	if strings.Contains(string(body), `phpinfo();`+CRLF) {
		panic("CRLF in Body")
	}
	if !strings.Contains(string(body), "phpinfo();") {
		panic("FAILED")
	}

	boundaryWithPrefix := `------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---`
	suffix := ` ?>` + CRLF + boundaryWithPrefix + "\r\n\r\n\n\r\n" + boundaryWithPrefix + "--\r\n"
	if !strings.HasSuffix(string(body), suffix) {
		spew.Dump(string(body))
		println("---------------------------")
		spew.Dump(suffix)
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
	if !strings.Contains(string(body), `Content-Disposition: form-data; name="{\"key\": \"value\"}"`+CRLF+CRLF+CRLF+"---------") {
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
	if !strings.Contains(string(body), `Content-Disposition: form-data; name="{\"key\": \"value\"}"`+CRLF+CRLF+"--------------------------123\r\n") {
		spew.Dump(string(body))
		panic("CRLF in form1")
	}

	if !strings.Contains(string(body), `Content-Disposition: form-data; name="{\"key\": \"value\"}"`+CRLF+CRLF+CRLF+"--------------------------123--") {
		panic("CRLF in form2")
	}
	// VerifyMultipart(b, body)
}

func TestFixMultipartPacket(t *testing.T) {
	// LS0tLS0tLS0tLS0tRWYxS003R0kzRWYxZWk0SWo1YWUwS003Y0gyS003DQpDb250ZW50LURpc3Bvc2l0aW9uOiBmb3JtLWRhdGE7IG5hbWU9ImZpbGUiOyBmaWxlbmFtZT0iYS5waHAiDQpDb250ZW50LVR5cGU6IGltYWdlL3BuZw0KDQo8P3BocCBwaHBpbmZvKCk7ID8+DQotLS0tLS0tLS0tLS1FZjFLTTdHSTNFZjFlaTRJajVhZTBLTTdjSDJLTTctLQ0K
	flag := `LS0tLS0tLS0tLS0tRWYxS003R0kzRWYxZWk0SWo1YWUwS003Y0gyS003DQpDb250ZW50LURpc3Bvc2l0aW9uOiBmb3JtLWRhdGE7IG5hbWU9ImZpbGUiOyBmaWxlbmFtZT0iYS5waHAiDQpDb250ZW50LVR5cGU6IGltYWdlL3BuZw0KDQo8P3BocCBwaHBpbmZvKCk7ID8+DQotLS0tLS0tLS0tLS1FZjFLTTdHSTNFZjFlaTRJajVhZTBLTTdjSDJLTTctLQ0K`
	raw, _ := codec.DecodeBase64(`LS0tLS0tLS0tLS0tRWYxS003R0kzRWYxZWk0SWo1YWUwS003Y0gyS003DQpDb250ZW50LURpc3Bvc2l0aW9uOiBmb3JtLWRhdGE7IG5hbWU9ImZpbGUiOyBmaWxlbmFtZT0iYS5waHAiDQpDb250ZW50LVR5cGU6IGltYWdlL3BuZw0KDQo8P3BocCBwaHBpbmZvKCk7ID8+DQotLS0tLS0tLS0tLS1FZjFLTTdHSTNFZjFlaTRJajVhZTBLTTdjSDJLTTctLQ0K`)
	spew.Dump(raw)
	_, raw = FixMultipartBody(raw)
	spew.Dump(raw)
	if flag != codec.EncodeBase64(raw) {
		panic(1)
	}
}

func TestFixMultipartSpecialCase(t *testing.T) {
	for _, c := range []struct {
		Expect string
		Input  string
	}{
		{Input: "--a\nA: Bar  \n--a--", Expect: "--a\r\nA: Bar  \r\n--a--\r\n"},
		{Input: "--a\nA:Bar  \n--a--", Expect: "--a\r\nA:Bar  \r\n--a--\r\n"},
		{Input: "--a\nA:Bar  \n--a--\r", Expect: "--a\r\nA:Bar  \r\n--a--\r\r\n--a--\r\n"},
		{Input: "--a\nA: Bar  \n--a--\n", Expect: "--a\r\nA: Bar  \r\n--a--\r\n"},
		{Input: "--a\nA: Bar  \n\n--a--\n", Expect: "--a\r\nA: Bar  \r\n\r\n--a--\r\n"},
		{Input: "--a\nA: Bar  \n\n\n--a--\n", Expect: "--a\r\nA: Bar  \r\n\r\n\r\n--a--\r\n"},
		{Input: "--a\nA: Bar  \n\n\n\n--a--\n", Expect: "--a\r\nA: Bar  \r\n\r\n\n\r\n--a--\r\n"},
		{Input: "--a\nA: Bar  \n\n" + "\n" + "\n--a--\n", Expect: "--a\r\nA: Bar  \r\n\r\n" + "\n" + "\r\n--a--\r\n"},
		{Input: "--a\nA: Bar  \n\n" + "\n\v\v\v\v\r\r" + "\n--a--\n", Expect: "--a\r\nA: Bar  \r\n\r\n" + "\n\v\v\v\v\r" + "\r\n--a--\r\n"},
		{Input: "--a\nA: Bar  \n\n" + "\n\r\n\r\r\r" + "\n--a--\n", Expect: "--a\r\nA: Bar  \r\n\r\n" + "\n\r\n\r\r" + "\r\n--a--\r\n"},
	} {
		_, raw := FixMultipartBody([]byte(c.Input))
		if c.Expect != string(raw) {
			spew.Dump(c.Expect)
			spew.Dump(string(raw))
			panic("FAILED")
		}
	}
}
