package yaklib

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestCsrfPOCGet(t *testing.T) {
	poc, err := GenerateCSRFPoc(`GET /get?a=1&a=2&b=c&d=e&%66=%67&h=/>&i="\" HTTP/1.1
Accept: */*
Accept-Encoding: gzip, deflate
Connection: keep-alive
Host: httpbin.org
User-Agent: HTTPie/3.2.1

`)
	t.Log(poc)
	spew.Dump(err)
}

func TestCsrfPOCPost(t *testing.T) {
	poc, err := GenerateCSRFPoc(`POST /post HTTP/1.1
Accept: */*
Accept-Encoding: gzip, deflate
Connection: keep-alive
Host: httpbin.org
User-Agent: HTTPie/3.2.1
Content-Type: application/x-www-form-urlencoded
Content-Length: 15

a[1]=1&a[2]=1&c=1&d=2
`)
	t.Log(poc)
	spew.Dump(err)
}

func TestCsrfPOCMultipartTrue(t *testing.T) {
	poc, err := GenerateCSRFPoc(`POST /post HTTP/1.1
Accept: */*
Accept-Encoding: gzip, deflate
Connection: keep-alive
Host: httpbin.org
User-Agent: HTTPie/3.2.1
Content-Type:multipart/form-data;boundary=----------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Length: 199

------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo(); ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7--
`, _csrfOptWithMultipartDefaultValue(true))
	t.Log(poc)
	spew.Dump(err)
}

func TestCsrfPOCMultipartFalse(t *testing.T) {
	poc, err := GenerateCSRFPoc(`POST /post HTTP/1.1
Accept: */*
Accept-Encoding: gzip, deflate
Connection: keep-alive
Host: httpbin.org
User-Agent: HTTPie/3.2.1
Content-Type:multipart/form-data;boundary=----------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Length: 199

------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="postfile"; filename="a.php"
Content-Type: image/png

<?php phpinfo(); ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7--
`)
	t.Log(poc)
	spew.Dump(err)
}
