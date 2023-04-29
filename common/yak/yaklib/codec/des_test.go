package codec

import (
	"github.com/davecgh/go-spew/spew"
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
