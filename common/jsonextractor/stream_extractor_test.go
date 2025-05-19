package jsonextractor

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type jsonStreamTestCase struct {
	name                 string
	raw                  string
	kvCallbackAssertions func(key, data any, keyMatch *bool, valMatch *bool, counter *int)
	expectKeyMatch       bool
	expectValMatch       bool
	expectCount          int // Expected number of times the callback is called.
}

func TestExtractJSONStream_TableDriven(t *testing.T) {
	testCases := []jsonStreamTestCase{
		{
			name: "Simple K/V pair (Original TestExtractJSONStream)",
			raw:  `{"abc"  :"abccc"}`,
			kvCallbackAssertions: func(key, data any, keyMatch *bool, valMatch *bool, counter *int) {
				if keyStr, ok := key.(string); ok && keyStr == `"abc"  ` {
					*keyMatch = true
				}
				if dataStr, ok := data.(string); ok && dataStr == `"abccc"` {
					*valMatch = true
				}
				if counter != nil {
					(*counter)++
				}
			},
			expectKeyMatch: true,
			expectValMatch: true,
			expectCount:    2,
		},
		{
			name: "K/V pair with array value (Original TestExtractJSONStreamArray)",
			raw:  `{"abc"  :["v1", "ccc", "eee"]}`,
			kvCallbackAssertions: func(key, data any, keyMatch *bool, valMatch *bool, counter *int) {
				if keyStr, ok := key.(string); ok && keyStr == `"abc"  ` {
					*keyMatch = true
				}
				// valMatch is not asserted to be true in the original test for array.
				if counter != nil {
					(*counter)++
				}
			},
			expectKeyMatch: true,
			expectValMatch: false, // Original test didn't require valPass to be true.
			expectCount:    5,
		},
		{
			name: "Multiple K/V pairs with count (Original TestExtractJSONStream2)",
			raw:  `{"abc"  :"abccc", "def" : "def"}`,
			kvCallbackAssertions: func(key, data any, keyMatch *bool, valMatch *bool, counter *int) {
				if keyStr, ok := key.(string); ok && keyStr == `"abc"  ` {
					*keyMatch = true
				}
				if dataStr, ok := data.(string); ok && dataStr == `"abccc"` {
					*valMatch = true
				}
				if counter != nil {
					(*counter)++
				}
			},
			expectKeyMatch: true,
			expectValMatch: true,
			expectCount:    3, // Based on original test's count assertion (N(N+1)/2 for N=2 keys)
		},
		{
			name: "More K/V pairs with count (Original TestExtractJSONStream3)",
			raw:  `{"abc"  :"abccc", "def" : "def", "ghi" : "ghi", "jkl" : "jkl"}`,
			kvCallbackAssertions: func(key, data any, keyMatch *bool, valMatch *bool, counter *int) {
				if keyStr, ok := key.(string); ok && keyStr == `"abc"  ` {
					*keyMatch = true
				}
				if dataStr, ok := data.(string); ok && dataStr == `"abccc"` {
					*valMatch = true
				}
				if counter != nil {
					(*counter)++
				}
			},
			expectKeyMatch: true,
			expectValMatch: true,
			expectCount:    5, // Based on N(N+1)/2 for N=4 keys, original was count > 2
		},
		{
			name: "Nested object 1 (Original TestExtractJSONStream_NEST1)",
			raw:  `{"abc"  :{"def" : "def"}}`,
			kvCallbackAssertions: func(key, data any, keyMatch *bool, valMatch *bool, counter *int) {
				if keyStr, ok := key.(string); ok && keyStr == `"def" ` { // Note the space
					*keyMatch = true
				}
				if dataStr, ok := data.(string); ok && dataStr == ` "def"` { // Note the space
					*valMatch = true
				}
				if counter != nil {
					(*counter)++
				}
				fmt.Println(key, data)

			},
			expectKeyMatch: true, // For inner key "def"
			expectValMatch: true, // For inner value "def"
			expectCount:    3,    // One callback for the inner pair
		},
		{
			name: "Nested object 2 with trailing space (Original TestExtractJSONStream_NEST2)",
			raw:  `{"abc"  :{"def" : "def"}  }`,
			kvCallbackAssertions: func(key, data any, keyMatch *bool, valMatch *bool, counter *int) {
				if keyStr, ok := key.(string); ok && keyStr == `"def" ` {
					*keyMatch = true
				}
				if dataStr, ok := data.(string); ok && dataStr == ` "def"` {
					*valMatch = true
				}
				if counter != nil {
					(*counter)++
				}
			},
			expectKeyMatch: true,
			expectValMatch: true,
			expectCount:    3,
		},
		{
			name: "Bad JSON 1 - extra quote in value (Original TestExtractJSONStream_BAD)",
			raw:  `{"abc"  :"abc"abc""  }`,
			kvCallbackAssertions: func(key, data any, keyMatch *bool, valMatch *bool, counter *int) {
				// Original test only cared about valPass
				if dataStr, ok := data.(string); ok && dataStr == `"abc"abc""  ` {
					*valMatch = true
				}
				// *keyMatch is not set, so actualKeyMatch will remain false.
				if counter != nil {
					(*counter)++
				}
			},
			expectKeyMatch: false, // keyPass was not asserted true in original
			expectValMatch: true,
			expectCount:    2,
		},
		{
			name: "Bad JSON 2 - missing quote in value (Original TestExtractJSONStream_BAD2)",
			raw:  `{"abc"  :"abc"abc"  }`,
			kvCallbackAssertions: func(key, data any, keyMatch *bool, valMatch *bool, counter *int) {
				// Original test only cared about valPass
				if dataStr, ok := data.(string); ok && dataStr == `"abc"abc"  ` {
					*valMatch = true
				}
				if counter != nil {
					(*counter)++
				}
			},
			expectKeyMatch: false, // keyPass was not asserted true in original
			expectValMatch: true,
			expectCount:    2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualKeyMatch := false
			actualValMatch := false
			actualCount := 0

			parseError := ExtractStructuredJSON(tc.raw, WithRawKeyValueCallback(func(key, data any) {
				tc.kvCallbackAssertions(key, data, &actualKeyMatch, &actualValMatch, &actualCount)
			}))
			if parseError != nil {
				if parseError != io.EOF {
					t.Fatal("SMOKING ERR: ", parseError)
				}
			}

			require.Equal(t, tc.expectKeyMatch, actualKeyMatch, "Key match expectation failed")
			require.Equal(t, tc.expectValMatch, actualValMatch, "Value match expectation failed")
			require.True(t, tc.expectCount <= actualCount, "Count expectation failed (number of callbacks)")
		})
	}
}

func TestStreamExtractorArray_SMOKING(t *testing.T) {
	ExtractStructuredJSON(`{a: []}`, WithRawKeyValueCallback(func(key, data any) {
		spew.Dump(key)
		spew.Dump(data)
	}))
}

func TestStreamExtractorArray_BASIC(t *testing.T) {
	keyHaveZero := false
	valueHaveResult := false
	ExtractStructuredJSON(`{a: ["abc"]}`, WithRawKeyValueCallback(func(key, data any) {
		if key == 0 {
			keyHaveZero = true
		}
		spew.Dump(data)
		if data == `"abc"` {
			valueHaveResult = true
		}
	}))
	assert.True(t, keyHaveZero)
	assert.True(t, valueHaveResult)
}

func TestStreamExtractorArray_BASIC2(t *testing.T) {
	keyHaveZero := false
	valueHaveResult := false
	ExtractStructuredJSON(`{a: ["abc"    ]}`, WithRawKeyValueCallback(func(key, data any) {
		if key == 0 {
			keyHaveZero = true
		}
		spew.Dump(data)
		if data == `"abc"    ` {
			valueHaveResult = true
		}
	}))
	assert.True(t, keyHaveZero)
	assert.True(t, valueHaveResult)
}

func TestStreamExtractorArray_BASIC3(t *testing.T) {
	keyHaveZero := false
	valueHaveResult := false
	emptyResult := false
	ExtractStructuredJSON(`{a: ["abc". ,    ]}`, WithRawKeyValueCallback(func(key, data any) {
		if key == 0 {
			keyHaveZero = true
		}
		spew.Dump(data)
		if data == `"abc". ` {
			valueHaveResult = true
		}
		if data == `    ` {
			emptyResult = true
		}
	}))
	assert.True(t, keyHaveZero)
	assert.True(t, valueHaveResult)
	assert.True(t, emptyResult)
}

func TestStreamExtractorArray_BASIC4(t *testing.T) {
	keyHaveZero := false
	valueHaveResult := false
	emptyResult := false
	ExtractStructuredJSON(`{a: ["abc". , ,,,,  ]}`, WithRawKeyValueCallback(func(key, data any) {
		if key == 0 {
			keyHaveZero = true
		}
		spew.Dump(data)
		if data == `"abc". ` {
			valueHaveResult = true
		}
		if data == `  ` {
			emptyResult = true
		}
	}))
	assert.True(t, keyHaveZero)
	assert.True(t, valueHaveResult)
	assert.True(t, emptyResult)
}
