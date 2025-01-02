package codec

import (
	"math/rand"
	"testing"

	uuid "github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

var LetterChar = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func checkAutoDecode(t *testing.T, text string, wants []string) {
	t.Helper()
	results := AutoDecode(text)
	require.Lenf(t, results, len(wants), "results[%v] length not match", results)

	for i := range results {
		require.Equal(t, wants[i], results[i].Result)
	}
}

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = LetterChar[rand.Intn(len(LetterChar))]
	}
	return string(b)
}

func TestAutoDecodeOneTime(t *testing.T) {
	t.Run("url decode", func(t *testing.T) {
		checkAutoDecode(t, `%65%78%61%6d%70%6c%65`, []string{"example"})
	})
	t.Run("html entity decode", func(t *testing.T) {
		checkAutoDecode(t, `&lt;script&gt;alert(1)&lt;/script&gt;`, []string{"<script>alert(1)</script>"})
	})
	t.Run("hex decode", func(t *testing.T) {
		checkAutoDecode(t, `68656c6c6f`, []string{"hello"})
	})
	t.Run("hex decode with 0x prefix", func(t *testing.T) {
		checkAutoDecode(t, `0x68656c6c6f`, []string{"hello"})
	})
	t.Run("unicode decode u", func(t *testing.T) {
		checkAutoDecode(t, `\u4f60\u597d`, []string{"你好"})
	})
	t.Run("unicode decode U", func(t *testing.T) {
		checkAutoDecode(t, `\U00004f60\U0000597d`, []string{"你好"})
	})
	t.Run("base64 decode", func(t *testing.T) {
		checkAutoDecode(t, `aGVsbG8=`, []string{"hello"})
	})
	t.Run("base32 decode", func(t *testing.T) {
		checkAutoDecode(t, `NBSWY3DP`, []string{"hello"})
	})
	t.Run("jwt decode", func(t *testing.T) {
		checkAutoDecode(t, `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c`, []string{`{"alg":"HS256","typ":"JWT"}.{"sub":"1234567890","name":"John Doe","iat":1516239022}.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c`})
	})
	t.Run("no", func(t *testing.T) {
		checkAutoDecode(t, `hello`, []string{"hello"})
	})
}

func TestAutoDecodeMultiTimes(t *testing.T) {
	t.Run("url-base64", func(t *testing.T) {
		checkAutoDecode(t, `aGVsbG8%3D`, []string{"aGVsbG8=", "hello"})
	})
	t.Run("html-url", func(t *testing.T) {
		checkAutoDecode(t, `&#104;&#101;&#108;&#108;&#111;&percnt;&#50;&#49;&percnt;&#52;&#48;&percnt;&#50;&#51;`, []string{"hello%21%40%23", "hello!@#"})
	})
	t.Run("hex-base64-html", func(t *testing.T) {
		checkAutoDecode(t, `4a694d784d4451374a694d784d4445374a694d784d4467374a694d784d4467374a694d784d5445374a6d5634593277374a6d4e76625731686444736d626e56744f773d3d`, []string{"JiMxMDQ7JiMxMDE7JiMxMDg7JiMxMDg7JiMxMTE7JmV4Y2w7JmNvbW1hdDsmbnVtOw==", "&#104;&#101;&#108;&#108;&#111;&excl;&commat;&num;", "hello!@#"})
	})
	t.Run("base64-jwt", func(t *testing.T) {
		checkAutoDecode(t, `ZXlKaGJHY2lPaUpJVXpJMU5pSXNJblI1Y0NJNklrcFhWQ0o5LmV5SnpkV0lpT2lJeE1qTTBOVFkzT0Rrd0lpd2libUZ0WlNJNklrcHZhRzRnUkc5bElpd2lhV0YwSWpveE5URTJNak01TURJeWZRLlNmbEt4d1JKU01lS0tGMlFUNGZ3cE1lSmYzNlBPazZ5SlZfYWRRc3N3NWM=`, []string{`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c`, `{"alg":"HS256","typ":"JWT"}.{"sub":"1234567890","name":"John Doe","iat":1516239022}.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c`})
	})

}

func TestAutoDecodeRandomString(t *testing.T) {
	encodeFuncMap := map[string]func(any) string{
		"url":  EncodeUrlCode,
		"html": EncodeHtmlEntity,
		"hex":  EncodeToHex,
		"unicode": func(s any) string {
			return JsonUnicodeEncode(string(interfaceToBytes(s)))
		},
		"base32": EncodeBase32,
		"base64": EncodeBase64,
	}
	keys := lo.Keys(encodeFuncMap)
	for i := 0; i < 10; i++ {
		randStr := uuid.NewString()
		randTimes := rand.Intn(2) + 3
		encodePaths := make([]string, 0, randTimes)
		encodePaths = append(encodePaths, randStr)
		for n := 0; n < randTimes; n++ {
			f := encodeFuncMap[keys[rand.Intn(len(encodeFuncMap))]]
			randStr = f(randStr)
			encodePaths = append(encodePaths, randStr)
		}
		wants := lo.Reverse(encodePaths)
		results := AutoDecode(randStr)
		gots := lo.Map(results, func(i *AutoDecodeResult, _ int) string { return i.Result })
		gots = append([]string{randStr}, gots...)
		require.Equal(t, wants, gots)
	}
}

func TestAutoDecode_BUG(t *testing.T) {
	t.Run("charset unexpected decode", func(t *testing.T) {
		testStr := `F8B4FC3F012D882489F98C2289583882789F9B1DA4A27EF15AFC28C412FCAF2629AC757AEDDE1DAE31415E792E8F8ACF2CEDAFFD84A9B085360A6E6ECF9852F0770DCA61452236B038C1953AD60E29B48794F9E6A178794175182E239500B81EC23C9AAB982471C18D6E41853843F6B3ABB1E96C201BA85A60534132ECF816DDEAFE052CA8496204C6634E81AF6508F2`

		want, err := DecodeHex(testStr)
		require.NoError(t, err)
		checkAutoDecode(t, testStr, []string{EscapeInvalidUTF8Byte(want)})
	})
}
