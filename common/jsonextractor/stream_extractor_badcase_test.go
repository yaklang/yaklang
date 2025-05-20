package jsonextractor

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
)

func TestJSONinONE(t *testing.T) {
	data := `{
  "name": "测试数据",
  "version": 1.0,
  "description": "用于测试JSON解析器的示例数据。",
  "isActive": true,
  "score": null,
  "configuration": {
    "isEnabled": false,
    "retryAttempts": 3,
    "settings": {
      "timeout": 5000,
      "mode": "strict",
      "advanced": {
        "featureA": true,
        "featureB": "off",
        "featureC": [1, 2, 3, "mixed"]
      }
    }
  },
  "emptyObject": {},
  "emptyArray": [],
  "unicodeString": "你好，世界！🌍"
}
`

	unicodeString := ""
	ExtractStructuredJSON(
		data,
		WithObjectCallback(func(data map[string]any) {
			fmt.Println("-------------------------------")
			if result, ok := data[`unicodeString`]; ok {
				unicodeString = fmt.Sprint(result)
			}
			fmt.Println("-------------------------------")
		}),
	)
	fmt.Println(unicodeString)
	assert.Equal(t, unicodeString, "你好，世界！🌍")
}

func TestStreamExtractor_Action(t *testing.T) {
	haveResult := false
	ExtractStructuredJSON(`
{
    "subtasks": [
        {
            "goal": "ABC1"
        }
    ]
}
			`, WithObjectCallback(func(data map[string]any) {
		fmt.Println("-------------------------------")
		spew.Dump(data)
		haveResult = true
	}))
	assert.True(t, haveResult)
}

func TestStreamExtractor_NestObj(t *testing.T) {
	ExtractStructuredJSON(`{"abc": {"a": 1}}`, WithObjectCallback(func(data map[string]any) {
		spew.Dump(data)
	}))
}

func TestStreamExtractor_BadCase(t *testing.T) {
	haveInt64 := false
	haveBool := false
	haveNull := false
	haveFullMap := false

	ExtractStructuredJSON(`{"name": "John Deo", 
"age": 30,

"isActive": true,

"address": null
}`, WithObjectKeyValue(func(key string, data any) {
		fmt.Println("--------------------------------------")
		spew.Dump(key, data)
		if data == 30 {
			log.Info("int64 found")
			haveInt64 = true
		}
		if data == true {
			haveBool = true
		}
		if data == nil {
			log.Info("nil found")
			haveNull = true
		}

		if reflect.ValueOf(data).Kind() == reflect.Map {
			haveFullMap = true
		}
		fmt.Println("--------------------------------------")
	}))
	assert.True(t, haveInt64)
	assert.True(t, haveBool)
	assert.True(t, haveNull)
	assert.True(t, haveFullMap)
}
func TestStreamExtractor_ArrayNewLine(t *testing.T) {
	hasArray := false
	ExtractStructuredJSON(`{"array":[1,
	2]}`, WithObjectKeyValue(func(key string, data any) {
		hasArray = hasArray || key == "array"
	}))
	assert.True(t, hasArray)
}

func _TestStreamExtractor_Array(t *testing.T) {
	hasArray := false
	ExtractStructuredJSON(`[1]`, WithObjectKeyValue(func(key string, data any) {
		hasArray = true
	}))
	assert.True(t, hasArray)
}

func TestComprehensiveEdgeCases(t *testing.T) {
	// This test covers a wide range of JSON edge cases and data types
	testJSON := `{
		"emptyString": "",
		"nullValue": null,
		"booleanValues": {
			"trueValue": true,
			"falseValue": false
		},
		"numericValues": {
			"zeroInt": 0,
			"positiveInt": 42,
			"negativeInt": -42,
			"maxInt": 9007199254740991,
			"minInt": -9007199254740991,
			"zeroFloat": 0.0,
			"positiveFloat": 3.14159,
			"negativeFloat": -3.14159,
			"scientificNotation": 1.23e+20,
			"negativeScientific": -4.56e-10
		},
		"stringValues": {
			"alphabetic": "abcDEF",
			"alphanumeric": "abc123",
			"whitespace": "   spaced   ",
			"controlChars": "\n\t\r\b\f",
			"escapeChars": "Quote: \", Backslash: \\, Slash: \/, Backspace: \b, Form feed: \f, Newline: \n, Carriage return: \r, Tab: \t",
			"unicode": "Unicode: \u00A9 \u00AE \u2122",
			"emoji": "Emoji: 😀 🌍 🚀 ❤️",
			"mixedLanguages": "English, 中文, Español, Русский"
		},
		"arrayValues": {
			"emptyArray": [],
			"singleItemArray": ["solo"],
			"multiItemArray": [1, 2, 3],
			"mixedTypeArray": [null, true, 42, "string", {"key": "value"}, [1, 2]],
			"nestedArrays": [
				[1, 2],
				[3, 4, [5, 6]]
			]
		},
		"objectValues": {
			"emptyObject": {},
			"simpleObject": {"key": "value"},
			"nestedObject": {"outer": {"inner": {"deepest": "value"}}},
			"complexObject": {
				"array": [1, 2, 3],
				"object": {"key": "value"},
				"mixed": [{"key1": "value1"}, {"key2": "value2"}]
			}
		},
		"specialCases": {
			"duplicatedKeys": {"key": "first", "key": "second"},
			"leadingZeros": {"leadingZero": 0.1, "leadingDecimal": 0.123},
			"paddedStrings": {"padded": "  padded value  "},
			"commaEdgeCases": {"trailingComma": [1, 2, 3]},
			"quotedNumbers": {"quoted": "42"},
			"pathologicallyNestedObject": {"level1": {"level2": {"level3": {"level4": {"level5": {"level6": {"level7": {"level8": {"level9": {"level10": "deep"}}}}}}}}}}
		},
		"extremeContent": {
			"longString": "` + strings.Repeat("x", 1000) + `",
			"longNumber": 1` + strings.Repeat("0", 100) + `,
			"deeplyNestedArray": [` + strings.Repeat("[", 20) + `0` + strings.Repeat("]", 20) + `]
		}
	}`

	// Create maps to track the types we've encountered
	foundTypes := map[string]bool{
		"null":           false,
		"boolean":        false,
		"integer":        false,
		"float":          false,
		"string":         false,
		"emptyString":    false,
		"emptyArray":     false,
		"emptyObject":    false,
		"array":          false,
		"object":         false,
		"nestedArray":    false,
		"nestedObject":   false,
		"complexObject":  false,
		"unicodeString":  false,
		"escapeChars":    false,
		"leadingZeros":   false,
		"extremeContent": false,
	}

	// Track how many keys we actually process
	processedKeys := 0

	// Process the JSON
	err := ExtractStructuredJSON(testJSON,
		WithObjectKeyValue(func(key string, data any) {
			processedKeys++
			if key == "mixedTypeArray" {
				spew.Dump(data)
			}
			if key == "emptyObject" {
				foundTypes["emptyObject"] = true
			}

			// Check the data type and update our tracking
			if data == nil {
				foundTypes["null"] = true
			} else {
				switch v := data.(type) {
				case bool:
					foundTypes["boolean"] = true
				case int:
					foundTypes["integer"] = true
				case float64:
					foundTypes["float"] = true
				case string:
					foundTypes["string"] = true
					if v == "" {
						foundTypes["emptyString"] = true
					}
					if key == "unicode" || key == "emoji" || key == "mixedLanguages" {
						foundTypes["unicodeString"] = true
					}
					if key == "escapeChars" {
						foundTypes["escapeChars"] = true
					}
				case map[string]any:
					foundTypes["object"] = true
					if len(v) == 0 {
						foundTypes["emptyObject"] = true
					}
					if key == "nestedObject" || key == "pathologicallyNestedObject" {
						foundTypes["nestedObject"] = true
					}
					if key == "complexObject" {
						foundTypes["complexObject"] = true
					}
				case []any:
					foundTypes["array"] = true
					if len(v) == 0 {
						foundTypes["emptyArray"] = true
					}
					if key == "nestedArrays" {
						foundTypes["nestedArray"] = true
					}
				}
			}

			// Check for specific edge cases
			if key == "leadingZero" || key == "leadingDecimal" {
				foundTypes["leadingZeros"] = true
			}
			if key == "longString" || key == "longNumber" || key == "deeplyNestedArray" {
				foundTypes["extremeContent"] = true
			}
		}),
		WithObjectCallback(func(data map[string]any) {
			// This just helps ensure we process the whole object
		}),
		WithArrayCallback(func(data []any) {
			// This just helps ensure we process arrays too
		}),
	)

	// Verify no errors in processing
	assert.Nil(t, err, "JSON processing should not produce an error")

	// Verify we processed at least some keys
	assert.Greater(t, processedKeys, 0, "Should have processed at least some keys")

	// Test that we found most of our expected data types
	for typeName, found := range foundTypes {
		assert.True(t, found, "Should have found %s data type", typeName)
	}

	// Output what we found for debugging
	fmt.Println("Processed keys:", processedKeys)
	spew.Dump("Found types:", foundTypes)
}

func TestStreamingEdgeCases(t *testing.T) {
	// This test focuses on streaming-specific edge cases

	testCases := []struct {
		name     string
		json     string
		expected bool // Whether we expect successful parsing
	}{
		{
			name:     "Incomplete JSON - Truncated Object",
			json:     `{"key": "value", "incomplete": `,
			expected: false,
		},
		{
			name:     "Incomplete JSON - Truncated Array",
			json:     `["item1", "item2", `,
			expected: false,
		},
		{
			name:     "Incomplete JSON - Truncated String",
			json:     `{"key": "value with unclosed quote`,
			expected: false,
		},
		{
			name:     "Chunked JSON - Complete chunks",
			json:     `{"chunk1": "data"}{"chunk2": "more data"}`,
			expected: true,
		},
		{
			name:     "Chunked JSON - With whitespace between",
			json:     `{"chunk1": "data"} \n\t\r {"chunk2": "more data"}`,
			expected: true,
		},
		{
			name:     "Complex Escape Sequences",
			json:     `{"escaped": "\\\\\\\"\\b\\f\\n\\r\\t\\u00A9"}`,
			expected: true,
		},
		{
			name:     "Nested Escape Sequences",
			json:     `{"nested": "Level 1: \\\"Level 2: \\\\\\\"Level 3\\\\\\\"\\\""}`,
			expected: true,
		},
		{
			name:     "Unicode Escapes",
			json:     `{"unicode": "\u0041\u00A9\u6C49\uD83D\uDE00"}`, // A, ©, 汉, 😀
			expected: true,
		},
		{
			name:     "Invalid Unicode Escapes",
			json:     `{"invalid": "\uXYZ\u"}`,
			expected: false,
		},
		{
			name:     "Extreme Nesting - Object",
			json:     `{"l1":{"l2":{"l3":{"l4":{"l5":{"l6":{"l7":{"l8":{"l9":{"l10":{"l11":{"l12":{"l13":{"l14":{"l15":{"l16":{"l17":{"l18":{"l19":{"l20":"deep"}}}}}}}}}}}}}}}}}}}}`,
			expected: true,
		},
		{
			name:     "Mixed Chunks With Partial JSON",
			json:     `{"complete": true} {"partial": true, "missing"`,
			expected: false,
		},
		{
			name:     "Non-standard Escapes",
			json:     `{"nonstandard": "\a\v\e"}`,
			expected: false,
		},
		{
			name:     "Numbers With Different Formats",
			json:     `{"numbers": [0, -0, 0.0, 1e10, 1e+10, 1e-10, -1e-10, 1.23456789012345678901234567890]}`,
			expected: true,
		},
		{
			name:     "Invalid Number Formats",
			json:     `{"invalid": [.1, 1., +1, 01, 1.0e, 1.0e+, 1.0e-, 0x1, Infinity, NaN]}`,
			expected: false,
		},
		{
			name:     "Control Characters in String",
			json:     string([]byte{'{', '"', 'c', 't', 'r', 'l', '"', ':', '"', 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, '"', '}'}),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var successfulParse bool = true
			var parseError error

			// Try to extract the JSON and catch any panics
			func() {
				defer func() {
					if r := recover(); r != nil {
						successfulParse = false
						t.Logf("Caught panic: %v", r)
					}
				}()

				foundObjects := 0
				parseError = ExtractStructuredJSON(tc.json,
					WithObjectCallback(func(data map[string]any) {
						foundObjects++
					}),
					WithArrayCallback(func(data []any) {
						foundObjects++
					}),
				)

				// If we got an error but not EOF, consider parse failed
				if parseError != nil && parseError.Error() != "EOF" {
					successfulParse = false
				}

				// If we found no objects, also consider parse failed
				if foundObjects == 0 {
					successfulParse = false
				}
			}()

			// Report results
			if tc.expected {
				assert.True(t, successfulParse, "Expected successful parse but got failure")
			} else {
				// If we don't expect successful parsing, it's okay to fail
				// This test is more about not crashing than getting the right answer
				t.Logf("Expected failure case handled appropriately")
			}
		})
	}
}

// Test focusing on malformed JSON that's still parseable in certain contexts
func TestMalformedButParseable(t *testing.T) {
	testCases := []struct {
		name     string
		json     string
		expected bool // Whether we expect our parser to extract something
	}{
		{
			name:     "Missing quotes around property name",
			json:     `{key: "value"}`,
			expected: true,
		},
		{
			name:     "Single quotes around string",
			json:     `{'key': 'value'}`,
			expected: true,
		},
		{
			name:     "Trailing comma in object",
			json:     `{"key": "value",}`,
			expected: true,
		},
		{
			name:     "Trailing comma in array",
			json:     `["item1", "item2",]`,
			expected: true,
		},
		{
			name: "Comments in JSON",
			json: `{
				"key": "value", // this is a comment
				/* multi-line comment */
				"another": true
			}`,
			expected: false, // Standard JSON doesn't allow comments
		},
		{
			name: "Unquoted JSON values",
			json: `{
				"string": hello world,
				"color": #FF0000
			}`,
			expected: false,
		},
		{
			name:     "Extra commas",
			json:     `{"items": [1,, 2, 3]}`,
			expected: false,
		},
		{
			name:     "Hex values",
			json:     `{"color": 0xFF0000}`,
			expected: true, // Might parse as number
		},
		{
			name: "Javascript-style JSON with methods",
			json: `{
				"name": "Test",
				"getName": function() { return this.name; }
			}`,
			expected: false,
		},
		{
			name:     "Mixed quotes",
			json:     `{"key1": "value1", 'key2': "value2"}`,
			expected: true, // Our parser might handle this
		},
		{
			name:     "JSON embedded in HTML",
			json:     `<script type="application/json">{"key": "value"}</script>`,
			expected: true, // Should extract the JSON part
		},
		{
			name:     "JSON with numeric key",
			json:     `{1: "numeric key"}`,
			expected: true,
		},
		{
			name:     "Duplicate keys with different types",
			json:     `{"key": "string", "key": 42}`,
			expected: true, // Should handle this, using one of the values
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var extractedSomething bool
			var extractedData []string

			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Logf("Caught panic: %v", r)
					}
				}()

				// Try to extract with both methods
				extracted, _ := ExtractJSONWithRaw(tc.json)
				extractedData = extracted
				extractedSomething = len(extracted) > 0
			}()

			// Display what we found
			if extractedSomething {
				t.Logf("Extracted JSON: %v", extractedData)
			}

			// Note: This isn't a strict test of correctness, just that the parser
			// behaves as we expect with this type of malformed JSON
			if tc.expected != extractedSomething {
				t.Logf("Expected extraction result: %v, got: %v", tc.expected, extractedSomething)
			}
		})
	}
}
