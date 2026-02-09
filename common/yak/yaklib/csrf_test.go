package yaklib

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
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
	require.NoError(t, err)
}

func TestCsrfPOCJSONPost(t *testing.T) {
	poc, err := GenerateCSRFPoc(`POST /post?a=1&b=2 HTTP/1.1
Accept: */*
Accept-Encoding: gzip, deflate
Connection: keep-alive
Host: httpbin.org
User-Agent: HTTPie/3.2.1
Content-Type: application/json
Content-Length: 16

{"key": "value"}
`)
	t.Log(poc)
	require.NoError(t, err)
}

func TestCsrfPOCMultipartForm(t *testing.T) {
	t.Run("use js template", func(t *testing.T) {
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
		`, CsrfOptWithMultipartDefaultValue(true))
		t.Log(poc)
		require.NoError(t, err)
	})
	t.Run("fix EOF error", func(t *testing.T) {
		poc, err := GenerateCSRFPoc(`POST / HTTP/1.1
Content-Type: multipart/form-data; boundary=63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618
Host: www.example.com
Content-Length: 183

--63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618
Content-Disposition: form-data; name="key"

value
--63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618--`)
		t.Log(poc)
		require.NoError(t, err)
	})

	t.Run("same key", func(t *testing.T) {
		poc, err := GenerateCSRFPoc(`POST / HTTP/1.1
Content-Type: multipart/form-data; boundary=63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618
Host: www.example.com
Content-Length: 183

--63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618
Content-Disposition: form-data; name="key"

value
--63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618
Content-Disposition: form-data; name="key"

value
--63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618--`)
		t.Log(poc)
		require.Equal(t, strings.Count(poc, `<input type="hidden" name="key" value="value"/>`), 2)
		require.NoError(t, err)
	})
}

func TestCsrfPOCAutoSubmitOption(t *testing.T) {
	raw := `POST /post HTTP/1.1
Host: httpbin.org
Content-Type: application/x-www-form-urlencoded
Content-Length: 7

foo=bar
`
	t.Run("auto submit disabled (default)", func(t *testing.T) {
		poc, err := GenerateCSRFPoc(raw)
		require.NoError(t, err)
		require.NotContains(t, poc, `document.forms['form1']`)
	})

	t.Run("auto submit enabled", func(t *testing.T) {
		poc, err := GenerateCSRFPoc(raw, CsrfOptWithAutoSubmit(true))
		require.NoError(t, err)
		require.Contains(t, poc, `document.forms['form1']`)
		require.Contains(t, poc, `HTMLFormElement.prototype.submit.call(f)`)
		// should still contain the form and foo field (value may include trailing newline from raw packet)
		require.Contains(t, poc, `name="foo"`)
		require.Contains(t, poc, `<form `)
	})
}
