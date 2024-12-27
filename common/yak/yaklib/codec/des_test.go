package codec

import (
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestDESCBCDec(t *testing.T) {
	origin, err := DESEncryptCBCWithZeroPadding(ZeroPadding([]byte("test"), 8), []byte("asdfasdfasdfsdfasdf"), nil)
	if err != nil {
		panic(err)
	}
	println(StrConvQuoteHex(string(origin)))

	data, err := DESDecryptCBCWithZeroPadding(ZeroPadding([]byte("test"), 8), origin, nil)
	if err != nil {
		panic(err)
	}
	println(StrConvQuoteHex(string(data)))
}

func TestDesECB(t *testing.T) {
	bytes, err := DESECBEnc(ZeroPadding([]byte(`abc`), 8), []byte(`abc`))
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(bytes))

	origin, err := DESECBDec(ZeroPadding([]byte(`abc`), 8), bytes)
	if err != nil {
		panic(err)
	}
	spew.Dump(origin)
}

func TestTripleDES_CBC(t *testing.T) {
	ede2Key := []byte("example key 1234")
	var tripleDESKey []byte
	tripleDESKey = append(tripleDESKey, ede2Key[:16]...)
	tripleDESKey = append(tripleDESKey, ede2Key[:8]...)

	plainText := "abc"

	bytes, err := TripleDESEncryptCBCWithZeroPadding(tripleDESKey, []byte(plainText), nil)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(bytes))

	origin, err := TripleDESDecryptCBCWithZeroPadding(tripleDESKey, bytes, nil)
	if err != nil {
		panic(err)
	}
	spew.Dump(origin)

	if strings.Trim(string(origin), "\x00") != plainText {
		panic("not expected")
	}
}

func TestTripleDES_ECB(t *testing.T) {
	ede2Key := []byte("example key 1234")
	var tripleDESKey []byte
	tripleDESKey = append(tripleDESKey, ede2Key[:16]...)
	tripleDESKey = append(tripleDESKey, ede2Key[:8]...)
	plainText := "abc"

	t.Run("enc", func(t *testing.T) {

		bytes, err := TripleDES_ECBEnc(tripleDESKey, []byte(plainText))
		require.NoError(t, err)

		origin, err := TripleDESDecFactory(ZeroUnPadding, ECB)(tripleDESKey, bytes, nil)
		require.NoError(t, err)
		spew.Dump(origin)
		require.Equal(t, plainText, strings.Trim(string(origin), "\x00"))
	})

	t.Run("dec", func(t *testing.T) {
		bytes, err := TripleDESEncFactory(ZeroPadding, ECB)(tripleDESKey, []byte(plainText), nil)
		require.NoError(t, err)
		spew.Dump(EncodeBase64(bytes))

		origin, err := TripleDES_ECBDec(tripleDESKey, bytes)
		require.NoError(t, err)
		spew.Dump(origin)
		require.Equal(t, plainText, strings.Trim(string(origin), "\x00"))
	})

}
