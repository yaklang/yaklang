package codec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBase64Padding(t *testing.T) {
	i := `MQ`
	result := AutoDecode(i)
	require.Len(t, result, 1, "base64 decode failed")
}

func TestHTTPChunkedDecode(t *testing.T) {
	text := "hellowasdfnasdjkfaskdklasdfkaskodfpoasdfpoasdofpasdfasdfaa"
	afterChunked := string(HTTPChunkedEncode([]byte(text)))
	println(afterChunked)

	res, err := HTTPChunkedDecode([]byte(afterChunked))
	require.NoError(t, err)
	require.Equal(t, text, string(res), "chunked decode failed")
}

func TestZeroPadding(t *testing.T) {
	require.Equal(t, "asdfasdfasdf123123123asaaa\x00\x00\x00\x00\x00\x00", string(ZeroPadding([]byte("asdfasdfasdf123123123asaaa"), 8)), "zero padding failed")
	require.Equal(t, "asdfasdfasdf123123123asaaa", string(ZeroUnPadding(ZeroPadding([]byte("asdfasdfasdf123123123asaaa"), 8))), "zero unpadding failed")
}

func TestHmacSha512(t *testing.T) {
	expected := "c4c9648c334666029bec087e085fcecdca34b1ee85626dac0337761f322599081f85b96ac919b85bb0b5357821d9fcf5ed02bc432129e6e7679d1e61643aef3d"
	require.Equal(t, expected, EncodeToHex(HmacSha512("abc", "aaa")), "HMAC-SHA512 encoding failed")
}
