package multipart

import (
	"github.com/stretchr/testify/assert"
	"io"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestReader_UnhealthyBody(t *testing.T) {
	body := "--a\r\nKey: Value\r\n--a--"
	reader := NewReaderWithString(body)
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	bodyRaw, _ := io.ReadAll(part)
	assert.True(t, part.NoBody())
	assert.True(t, part.NoEmptyLineDivider())
	assert.Equal(t, "", string(bodyRaw))
}

func TestReader_UnhealthyBody2(t *testing.T) {
	body := "--a\r\nKey: Value\r\n\r\n--a--"
	reader := NewReaderWithString(body)
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	bodyRaw, _ := io.ReadAll(part)
	assert.True(t, part.NoBody())
	assert.False(t, part.NoEmptyLineDivider())
	assert.Equal(t, "", string(bodyRaw))
}

func TestReader(t *testing.T) {
	testWithCallBack := func(t *testing.T, body string, callback func(t *testing.T, index int, body string, part *Part)) {
		t.Helper()

		reader := NewReaderWithString(body)
		index := 0
		for {
			part, ferr := reader.NextPart()

			if part != nil {
				body, err := io.ReadAll(part)
				require.NoError(t, err)

				bodyString := string(body)
				spew.Dump(body)
				callback(t, index, bodyString, part)
			}

			if part == nil {
				break
			}
			require.NoError(t, ferr)
			index++
		}
	}

	t.Run("normal", func(t *testing.T) {
		testWithCallBack(t, `------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo(); ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7--
`, func(t *testing.T, index int, body string, part *Part) {
			require.Contains(t, string(body), "phpinfo();")
		})
	})
	t.Run("header", func(t *testing.T) {
		testWithCallBack(t, `--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"

--------------------------123--`, func(t *testing.T, index int, body string, part *Part) {
			require.Contains(t, part.GetHeader("Content-Disposition"), "form-data; name=\"{\\\"key\\\": \\\"value\\\"}\"")
		})
	})

	t.Run("multi", func(t *testing.T) {
		testWithCallBack(t, `--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"
--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"


--------------------------123--`, func(t *testing.T, index int, body string, part *Part) {
			require.Contains(t, part.GetHeader("Content-Disposition"), "form-data; name=\"{\\\"key\\\": \\\"value\\\"}\"")
			require.Empty(t, body)
		})
	})

	t.Run("LF in body", func(t *testing.T) {
		testWithCallBack(t, `------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();`+"\n"+` ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7--
`, func(t *testing.T, index int, body string, part *Part) {
			require.NotContains(t, body, "phpinfo();\r\n")
			require.Contains(t, string(body), "phpinfo();\n")
		})
	})

	t.Run("missing boundary", func(t *testing.T) {
		testWithCallBack(t, `------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();`+"\n"+` ?>
------------Ef1KM7GI3Ef1ei4Ij5ae
		`, func(t *testing.T, index int, body string, part *Part) {
			require.NotContains(t, body, "phpinfo();\r\n")
			require.Contains(t, string(body), "phpinfo();")
			require.Contains(t, string(body), "--Ef1KM7GI3Ef1ei4Ij5ae")
		})
	})

	t.Run("missing boundary2", func(t *testing.T) {
		testWithCallBack(t, `------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();`+"\n"+` ?>
-------`, func(t *testing.T, index int, body string, part *Part) {
			require.NotContains(t, body, "phpinfo();\r\n")
			require.Contains(t, string(body), "phpinfo();\n")
			require.Contains(t, string(body), "-------")
		})
	})

	t.Run("boundary suffix with `-`", func(t *testing.T) {
		testWithCallBack(t, `------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();`+"\n"+` ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7-----
		`, func(t *testing.T, index int, body string, part *Part) {
			require.NotContains(t, body, "phpinfo();\r\n")
			require.Contains(t, string(body), "phpinfo();\n")
		})
	})

	t.Run("suffix with multi empty line", func(t *testing.T) {
		testWithCallBack(t, `------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7---
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

<?php phpinfo();`+"\n"+` ?>
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7-----



`, func(t *testing.T, index int, body string, part *Part) {
			require.NotContains(t, body, "phpinfo();\r\n")
			require.Contains(t, string(body), "phpinfo();\n")
		})
	})
}
