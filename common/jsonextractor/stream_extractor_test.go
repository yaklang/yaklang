package jsonextractor

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestExtractJSONStream(t *testing.T) {
	raw := `{"abc"  :"abccc"}`
	keyPass := false
	valPass := false
	results := ExtractJSONStream(raw, WithKeyValueCallback(func(key, data any) {
		if key == `"abc"  ` {
			keyPass = true
		}
		if data == `"abccc"` {
			valPass = true
		}
	}))
	require.Greater(t, len(results), 0)
	require.True(t, keyPass)
	require.True(t, valPass)
}

func TestExtractJSONStreamArray(t *testing.T) {
	raw := `{"abc"  :["v1", "ccc", "eee"]}`
	keyPass := false
	valPass := false
	results := ExtractJSONStream(raw, WithKeyValueCallback(func(key, data any) {
		if key == `"abc"  ` {
			keyPass = true
		}
	}))
	require.Greater(t, len(results), 0)
	require.True(t, keyPass)
	_ = valPass
}

func TestExtractJSONStream2(t *testing.T) {
	raw := `{"abc"  :"abccc", "def" : "def"}`
	keyPass := false
	valPass := false
	count := 0
	results := ExtractJSONStream(raw, WithKeyValueCallback(func(key, data any) {
		count++
		fmt.Println("--------------------------------")
		fmt.Printf("key: %#v value: %#v\n", key, data)
		fmt.Println("--------------------------------")
		if key == `"abc"  ` {
			keyPass = true
		}
		if data == `"abccc"` {
			valPass = true
		}
	}))
	require.Equal(t, 3, count)
	require.Greater(t, len(results), 0)
	require.True(t, keyPass)
	require.True(t, valPass)
}

func TestExtractJSONStream3(t *testing.T) {
	raw := `{"abc"  :"abccc", "def" : "def", "ghi" : "ghi", "jkl" : "jkl"}`
	keyPass := false
	valPass := false
	count := 0
	results := ExtractJSONStream(raw, WithKeyValueCallback(func(key, data any) {
		count++
		fmt.Println("--------------------------------")
		fmt.Println(key, data)
		fmt.Println("--------------------------------")
		if key == `"abc"  ` {
			keyPass = true
		}
		if data == `"abccc"` {
			valPass = true
		}
	}))
	require.Greater(t, count, 2)
	require.Greater(t, len(results), 0)
	require.True(t, keyPass)
	require.True(t, valPass)
}

func TestExtractJSONStream_NEST1(t *testing.T) {
	raw := `{"abc"  :{"def" : "def"}}`
	keyPass := false
	valPass := false
	results := ExtractJSONStream(raw, WithKeyValueCallback(func(key, data any) {
		fmt.Println("--------------------------------")
		fmt.Println(key, data)
		fmt.Println("--------------------------------")
		if key == `"def" ` {
			keyPass = true
		}
		if data == ` "def"` {
			valPass = true
		}
	}))
	require.Greater(t, len(results), 0)
	require.True(t, keyPass)
	require.True(t, valPass)
}

func TestExtractJSONStream_NEST2(t *testing.T) {
	raw := `{"abc"  :{"def" : "def"}  }`
	keyPass := false
	valPass := false
	results := ExtractJSONStream(raw, WithKeyValueCallback(func(key, data any) {
		fmt.Println("--------------------------------")
		fmt.Println(key, data)
		fmt.Println("--------------------------------")
		if key == `"def" ` {
			keyPass = true
		}
		if data == ` "def"` {
			valPass = true
		}
	}))
	require.Greater(t, len(results), 0)
	require.True(t, keyPass)
	require.True(t, valPass)
	spew.Dump(results)
}

func TestExtractJSONStream_BAD(t *testing.T) {
	raw := `{"abc"  :"abc"abc""  }`
	valPass := false
	results := ExtractJSONStream(raw, WithKeyValueCallback(func(key, data any) {
		fmt.Println("--------------------------------")
		fmt.Println(key, data)
		if data == `"abc"abc""  ` {
			valPass = true
		}
		fmt.Println("--------------------------------")
	}))
	require.Greater(t, len(results), 0)
	require.True(t, valPass)
	spew.Dump(results)
}

func TestExtractJSONStream_BAD2(t *testing.T) {
	raw := `{"abc"  :"abc"abc"  }`
	valPass := false
	results := ExtractJSONStream(raw, WithKeyValueCallback(func(key, data any) {
		fmt.Println("--------------------------------")
		fmt.Println(key, data)
		if data == `"abc"abc"  ` {
			valPass = true
		}
		fmt.Println("--------------------------------")
	}))
	require.Greater(t, len(results), 0)
	require.True(t, valPass)
	spew.Dump(results)
}
