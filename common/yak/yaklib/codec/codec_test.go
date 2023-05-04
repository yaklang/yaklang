package codec

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

func TestHTTPChunkedDecode(t *testing.T) {
	test := assert.New(t)

	// hellowasdfnasdjkfaskdklasdfkaskodfpoasfpoasdofpasdfasdfaa
	// hellowasdfnasdjkfaskdklasdfkaskodfpoasfpoasdofpasdfasdfaa

	// hellowasdfnasdjkfaskdklasdfkaskodfpoasdfpoasdofpasdfasdfaa
	text := "hellowasdfnasdjkfaskdklasdfkaskodfpoasdfpoasdofpasdfasdfaa"
	afterChunked := string(HTTPChunkedEncode([]byte(text)))

	println(afterChunked)
	res, err := HTTPChunkedDecode([]byte(afterChunked))
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	if text != string(res) {
		println(string(res))
		test.FailNow("chunked decode failed")
		return
	}
}

func TestZeroPadding(t *testing.T) {
	println(strconv.Quote(string(ZeroPadding([]byte("asdfasdfasdf123123123asaaa"), 8))))
	println(strconv.Quote(string(ZeroUnPadding(ZeroPadding([]byte("asdfasdfasdf123123123asaaa"), 8)))))
}

func TestHmacSha512(t *testing.T) {
	spew.Dump(EncodeToHex(HmacSha512("abc", "aaa")))
}
