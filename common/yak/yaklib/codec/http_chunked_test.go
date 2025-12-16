package codec

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	uuid "github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNotFullHTTPChunkedRead(t *testing.T) {
	bytes, rest := readHTTPChunkedData([]byte(`5
aaaaa
6
bbb`))
	require.Equal(t, "aaaaabbb", string(bytes))
	require.Len(t, rest, 0)
}

func TestHTTPChunkedRead(t *testing.T) {
	block, data := readHTTPChunkedData([]byte(`1
a
0

aaaacccc`))
	if string(block) != "a" {
		t.Fatal("block should not be a")
	}
	if string(data) != "aaaacccc" {
		t.Fatal("data should be aaaacccc")
	}
	block, data = readHTTPChunkedData([]byte(`11
aaaabbbbddddeeeef
2
ff
1
f
0

aaaacccc`))
	if string(block) != "aaaabbbbddddeeeeffff" {
		t.Fatal("block should not be a")
	}
	if string(data) != "aaaacccc" {
		t.Fatal("data should be aaaacccc")
	}

	block, data = readHTTPChunkedData([]byte(`11;aaaaaajnkasdjkfqwpjoe
aaaabbbbddddeeeef
2;jkasdjiof
ff
1;asdfpjkaspjodf
f
0

aaaacccc`))
	if string(block) != "aaaabbbbddddeeeeffff" {
		t.Fatal("block should not be a")
	}
	if string(data) != "aaaacccc" {
		t.Fatal("data should be aaaacccc")
	}

	rand1 := "asdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffff"
	rand2 := uuid.New().String()
	results := string(HTTPChunkedEncode([]byte(rand1))) + string(HTTPChunkedEncode([]byte(rand2)))
	rand1a, res := readHTTPChunkedData([]byte(results))
	spew.Dump(res)
	if string(rand1a) != rand1 {
		t.Fatal("rand1a should be rand1")
	}
	rand2a, res := readHTTPChunkedData(res)
	spew.Dump(res)
	if string(rand2a) != rand2 {
		t.Fatal("rand2a should be rand2")
	}
}

func TestHTTPChunkedRejectHugeChunkSize(t *testing.T) {
	// Similar to fuzz-tag newline injection:
	// 3\n a=:\n FFFFFFFFFFFF\n ... (attempt to allocate an enormous chunk)
	_, _, _, err := ReadHTTPChunkedDataWithFixedError([]byte("3\na=:\nFFFFFFFFFFFF\n0\n\n"))
	require.Error(t, err)
}
