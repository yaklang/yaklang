package codec

import (
	"github.com/davecgh/go-spew/spew"
	"strings"
	"testing"
)

func TestDESCBCDec(t *testing.T) {
	origin, err := DESCBCEnc([]byte("test"), ZeroPadding([]byte("asdfasdfasdfsdfasdf"), 8), nil)
	if err != nil {
		panic(err)
	}
	println(StrConvQuote(string(origin)))

	data, err := DESCBCDec([]byte("test"), origin, nil)
	if err != nil {
		panic(err)
	}
	println(StrConvQuote(string(data)))
}

func TestDesECB(t *testing.T) {
	bytes, err := DESECBEnc([]byte(`abc`), []byte(`abc`))
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(bytes))

	origin, err := DESECBDec([]byte(`abc`), bytes)
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

	bytes, err := TripleDES_CBCEnc(tripleDESKey, []byte(plainText), nil)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(bytes))

	origin, err := TripleDES_CBCDec(tripleDESKey, bytes, nil)
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
	bytes, err := TripleDES_ECBEnc(tripleDESKey, []byte(plainText))
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(bytes))

	origin, err := TripleDES_ECBDec(tripleDESKey, bytes)
	if err != nil {
		panic(err)
	}
	spew.Dump(origin)
	if strings.Trim(string(origin), "\x00") != plainText {
		panic("not expected")
	}
}
