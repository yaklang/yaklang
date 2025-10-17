package jsonextractor

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"

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
		if data == `"abc". ` {
			valueHaveResult = true
		}
	}), WithArrayCallback(func(data []any) {
		spew.Dump(data)
		for _, i := range data {
			if fmt.Sprint(i) == "" {
				emptyResult = true
			}
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
	}), WithArrayCallback(func(data []any) {
		for _, i := range data {
			if fmt.Sprint(i) == "" {
				emptyResult = true
			}
		}
	}))
	assert.True(t, keyHaveZero)
	assert.True(t, valueHaveResult)
	assert.True(t, emptyResult)
}

func TestStreamExtractorArray_BASIC5(t *testing.T) {
	resultLengthCheck := false
	allEmptyCheck := false
	ExtractStructuredJSON(`{a: [ ,    ]}`, WithArrayCallback(func(data []any) {
		spew.Dump(data)
		resultLengthCheck = len(data) == 2
		for _, d := range data {
			if fmt.Sprint(d) == "" {
				allEmptyCheck = true
			} else {
				allEmptyCheck = false
			}
		}
	}))
	assert.True(t, resultLengthCheck)
	assert.True(t, allEmptyCheck)
}

func TestStreamExtractorArray_BASIC6(t *testing.T) {
	resultLengthCheck := false
	ExtractStructuredJSON(`
[
{
"a":"b"
},
{
"b":"c"
},
{
"c":"a"
}]`, WithArrayCallback(func(data []any) {
		spew.Dump(data)
		resultLengthCheck = len(data) == 3
	}))
	assert.True(t, resultLengthCheck)
}

func TestStreamExtractorArray_BASIC7(t *testing.T) {
	resultLengthCheck := false
	ExtractStructuredJSON(` [
      {
        "value": "recon"
      }
    ]
`, WithArrayCallback(func(data []any) {
		spew.Dump(data)
		resultLengthCheck = len(data) == 1
	}))
	assert.True(t, resultLengthCheck)
}

func TestStreamExtractor_BASIC8(t *testing.T) {
	resultLengthCheck := false
	var result map[string]interface{}
	raw := `json { "@action": "continue-current-task", "status_summary": "当前任务状态：已测试 '<test>"' 的回显情况，JavaScript 输出未被编码或过滤。下一步将测试其他特殊字符（如 >, & 等）的回显情况，以确认是否存在更复杂的过滤机制。", "summary_tool_call_result": "使用 send_http_request_by_url 向 name 参数注入了特殊字符组合 '<test>\"'，返回的 HTML 页面中 JavaScript 成功输出原始字符串，未发现明显编码或过滤痕迹。" } `
	err := ExtractStructuredJSON(raw, WithObjectCallback(func(data map[string]any) {
		resultLengthCheck = len(data) == 3
		result = data
	}))
	require.NoError(t, err)
	spew.Dump(result)
	require.True(t, resultLengthCheck)
	assert.Equal(t, "continue-current-task", result["@action"])
	assert.Equal(t, "当前任务状态：已测试 '<test>\"' 的回显情况，JavaScript 输出未被编码或过滤。下一步将测试其他特殊字符（如 >, & 等）的回显情况，以确认是否存在更复杂的过滤机制。", result["status_summary"])
	assert.Equal(t, "使用 send_http_request_by_url 向 name 参数注入了特殊字符组合 '<test>\"'，返回的 HTML 页面中 JavaScript 成功输出原始字符串，未发现明显编码或过滤痕迹。", result["summary_tool_call_result"])
}

func TestStreamExtractor_BASIC9(t *testing.T) {
	raw := `{"@action": "require-tool", "tool": "now"}`
	ExtractStructuredJSON(raw, WithObjectCallback(func(data map[string]any) {
		spew.Dump(data)
	}))

}

func TestWithRegisterFieldStreamHandler(t *testing.T) {
	jsonData := `{
		"name": "John Doe",
		"data": "This is some streaming data content that should be passed to the handler",
		"age": 30
	}`

	var receivedData []byte
	dataReceived := false

	var wg sync.WaitGroup
	wg.Add(1)

	err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler("data", func(key string, reader io.Reader, parents []string) {
		defer wg.Done()
		data, readErr := io.ReadAll(reader)
		require.NoError(t, readErr)
		receivedData = data
		dataReceived = true
	}))

	require.NoError(t, err)
	wg.Wait() // 等待流处理完成
	assert.True(t, dataReceived, "Data should have been received through stream handler")
	assert.Contains(t, string(receivedData), "This is some streaming data content", "Received data should contain expected content")
}

func TestWithRegisterFieldStreamHandler_MultipleFields(t *testing.T) {
	jsonData := `{
		"field1": "Data for field 1",
		"field2": "Data for field 2",
		"field3": "Data for field 3"
	}`

	var field1Data, field2Data string
	var field1Received, field2Received bool
	var wg sync.WaitGroup
	var mu sync.Mutex

	wg.Add(2) // 等待两个字段处理完成

	err := ExtractStructuredJSON(jsonData,
		WithRegisterFieldStreamHandler("field1", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			mu.Lock()
			field1Data = string(data)
			field1Received = true
			mu.Unlock()
			fmt.Printf("Field1 received: %s\n", field1Data)
		}),
		WithRegisterFieldStreamHandler("field2", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			mu.Lock()
			field2Data = string(data)
			field2Received = true
			mu.Unlock()
			fmt.Printf("Field2 received: %s\n", field2Data)
		}),
	)

	require.NoError(t, err)

	// 等待goroutines完成
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// 所有goroutines完成
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for field stream handlers")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, field1Received, "Field1 should have been received")
	assert.True(t, field2Received, "Field2 should have been received")
	assert.Contains(t, field1Data, "Data for field 1")
	assert.Contains(t, field2Data, "Data for field 2")
}

func TestWithRegisterFieldStreamHandler_LargeData(t *testing.T) {
	// 测试大数据量的流式处理
	largeContent := strings.Repeat("Large streaming content data ", 1000)
	jsonData := fmt.Sprintf(`{"large_field": "%s"}`, largeContent)

	receivedSize := 0

	err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler("large_field", func(key string, reader io.Reader, parents []string) {
		buffer := make([]byte, 1024)
		for {
			n, readErr := reader.Read(buffer)
			if n > 0 {
				receivedSize += n
			}
			if readErr == io.EOF {
				break
			}
			require.NoError(t, readErr)
		}
	}))

	require.NoError(t, err)
	assert.Greater(t, receivedSize, 1000, "Should have received substantial amount of data")
}

func TestWithRegisterFieldStreamHandler_CharacterByCharacter(t *testing.T) {
	// 测试字符级流式处理，验证数据是逐字符写入的
	jsonData := `{"streaming_field": "Hello World 123"}`

	var receivedChars []byte
	var timestamps []time.Time
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(1)

	err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler("streaming_field", func(key string, reader io.Reader, parents []string) {
		defer wg.Done()
		buffer := make([]byte, 1)
		for {
			n, readErr := reader.Read(buffer)
			if n > 0 {
				mu.Lock()
				receivedChars = append(receivedChars, buffer[0])
				timestamps = append(timestamps, time.Now())
				mu.Unlock()
			}
			if readErr == io.EOF {
				break
			}
			require.NoError(t, readErr)
		}
	}))

	require.NoError(t, err)

	// 等待处理完成
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// 处理完成
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for stream processing")
	}

	mu.Lock()
	defer mu.Unlock()

	// 验证收到的数据
	expectedContent := `"Hello World 123"`
	assert.Equal(t, expectedContent, string(receivedChars), "Should receive complete field content character by character")
	assert.Greater(t, len(timestamps), 10, "Should have multiple character write timestamps")

	// 验证是流式处理（每个字符都有时间戳记录）
	assert.Equal(t, len(receivedChars), len(timestamps), "Each character should have a timestamp")
}

func TestWithRegisterFieldStreamHandler_StreamingOrder(t *testing.T) {
	// 测试流式处理的时序
	jsonData := `{"field1": "ABCDEFGHIJKLMNOP", "field2": "1234567890"}`

	var field1Chars []byte
	var field2Chars []byte
	var field1Times []time.Time
	var field2Times []time.Time
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(2)

	err := ExtractStructuredJSON(jsonData,
		WithRegisterFieldStreamHandler("field1", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			buffer := make([]byte, 1)
			for {
				n, readErr := reader.Read(buffer)
				if n > 0 {
					mu.Lock()
					field1Chars = append(field1Chars, buffer[0])
					field1Times = append(field1Times, time.Now())
					mu.Unlock()
				}
				if readErr == io.EOF {
					break
				}
				require.NoError(t, readErr)
			}
		}),
		WithRegisterFieldStreamHandler("field2", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			buffer := make([]byte, 1)
			for {
				n, readErr := reader.Read(buffer)
				if n > 0 {
					mu.Lock()
					field2Chars = append(field2Chars, buffer[0])
					field2Times = append(field2Times, time.Now())
					mu.Unlock()
				}
				if readErr == io.EOF {
					break
				}
				require.NoError(t, readErr)
			}
		}),
	)

	require.NoError(t, err)

	// 等待处理完成
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// 处理完成
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for stream processing")
	}

	mu.Lock()
	defer mu.Unlock()

	// 验证数据完整性
	assert.Equal(t, `"ABCDEFGHIJKLMNOP"`, string(field1Chars))
	assert.Equal(t, `"1234567890"`, string(field2Chars))

	// 验证field1在field2之前开始（因为在JSON中field1出现在前面）
	if len(field1Times) > 0 && len(field2Times) > 0 {
		assert.True(t, field1Times[0].Before(field2Times[0]) || field1Times[0].Equal(field2Times[0]),
			"Field1 should start before or at the same time as field2")
	}
}

// === 边界情况和错误处理测试 ===

func TestWithRegisterFieldStreamHandler_BoundaryConditions(t *testing.T) {
	t.Parallel()
	t.Run("Empty JSON", func(t *testing.T) {
		handlerCalled := false
		err := ExtractStructuredJSON(`{}`, WithRegisterFieldStreamHandler("nonexistent", func(key string, reader io.Reader, parents []string) {
			handlerCalled = true
		}))
		require.NoError(t, err)
		assert.False(t, handlerCalled, "Handler should not be called for non-existent field")
	})

	t.Run("Empty Field Value", func(t *testing.T) {
		var receivedData []byte
		handlerCalled := false
		var wg sync.WaitGroup
		wg.Add(1)

		err := ExtractStructuredJSON(`{"empty": ""}`, WithRegisterFieldStreamHandler("empty", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			handlerCalled = true
			data, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			receivedData = data
		}))

		require.NoError(t, err)
		wg.Wait() // 等待流处理完成
		assert.True(t, handlerCalled, "Handler should be called for empty field")
		assert.Equal(t, `""`, string(receivedData))
	})

	t.Run("Null Field Value", func(t *testing.T) {
		handlerCalled := false
		var receivedData []byte
		var wg sync.WaitGroup
		wg.Add(1)

		err := ExtractStructuredJSON(`{"nullfield": null}`, WithRegisterFieldStreamHandler("nullfield", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			handlerCalled = true
			data, _ := io.ReadAll(reader)
			receivedData = data
		}))
		require.NoError(t, err)
		wg.Wait() // 等待流处理完成

		// 字段流会被创建，null值应该被写入原始JSON数据
		if handlerCalled {
			assert.Equal(t, "null", string(receivedData), "Should receive 'null' for null field")
		}
	})

	t.Run("Numeric Field Value", func(t *testing.T) {
		handlerCalled := false
		var receivedData []byte
		var wg sync.WaitGroup
		wg.Add(1)

		err := ExtractStructuredJSON(`{"number": 12345}`, WithRegisterFieldStreamHandler("number", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			handlerCalled = true
			data, _ := io.ReadAll(reader)
			receivedData = data
		}))
		require.NoError(t, err)
		wg.Wait() // 等待流处理完成
		// 字段流会被创建，数字值应该被写入原始JSON数据
		if handlerCalled {
			assert.Equal(t, "12345", string(receivedData), "Should receive '12345' for numeric field")
		}
	})

	t.Run("Boolean Field Value", func(t *testing.T) {
		handlerCalled := false
		var receivedData []byte
		var wg sync.WaitGroup
		wg.Add(1)

		err := ExtractStructuredJSON(`{"flag": true}`, WithRegisterFieldStreamHandler("flag", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			handlerCalled = true
			data, _ := io.ReadAll(reader)
			receivedData = data
		}))
		require.NoError(t, err)
		wg.Wait() // 等待流处理完成
		// 字段流会被创建，布尔值应该被写入原始JSON数据
		if handlerCalled {
			assert.Equal(t, "true", string(receivedData), "Should receive 'true' for boolean field")
		}
	})
}

func TestWithRegisterFieldStreamHandler_MalformedJSON(t *testing.T) {
	t.Run("Incomplete JSON", func(t *testing.T) {
		incompleteJSON := `{"field": "start`
		handlerCalled := false

		err := ExtractStructuredJSON(incompleteJSON, WithRegisterFieldStreamHandler("field", func(key string, reader io.Reader, parents []string) {
			handlerCalled = true
		}))

		// 解析可能失败或成功，但不应该panic
		if handlerCalled {
			// 如果handler被调用了，说明部分数据被处理了
			t.Log("Handler was called with incomplete data")
		}
		t.Logf("Parse result: %v", err)
	})

	t.Run("Broken String Escape", func(t *testing.T) {
		brokenJSON := `{"field": "value with \\invalid escape"}`
		var receivedData string
		handlerCalled := false

		var wg sync.WaitGroup
		wg.Add(1)

		err := ExtractStructuredJSON(brokenJSON, WithRegisterFieldStreamHandler("field", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			handlerCalled = true
			data, _ := io.ReadAll(reader)
			receivedData = string(data)
		}))

		require.NoError(t, err)
		wg.Wait() // 等待流处理完成
		assert.True(t, handlerCalled, "Handler should be called even with escape issues")
		assert.Contains(t, receivedData, "invalid escape")
	})

	t.Run("Very Long Field Name", func(t *testing.T) {
		longFieldName := strings.Repeat("a", 10000)
		jsonData := fmt.Sprintf(`{"%s": "value"}`, longFieldName)

		handlerCalled := false
		err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler(longFieldName, func(key string, reader io.Reader, parents []string) {
			handlerCalled = true
		}))

		require.NoError(t, err)
		assert.True(t, handlerCalled, "Handler should work with very long field names")
	})
}

func TestWithRegisterFieldStreamHandler_ConcurrencyStress(t *testing.T) {
	t.Run("High Concurrency Field Processing", func(t *testing.T) {
		// 创建包含多个字段的JSON
		numFields := 50
		fieldData := make(map[string]string)
		jsonParts := []string{"{"}

		for i := 0; i < numFields; i++ {
			fieldName := fmt.Sprintf("field%d", i)
			fieldValue := strings.Repeat(fmt.Sprintf("data%d", i), 100)
			fieldData[fieldName] = fieldValue

			if i > 0 {
				jsonParts = append(jsonParts, ",")
			}
			jsonParts = append(jsonParts, fmt.Sprintf(`"%s": "%s"`, fieldName, fieldValue))
		}
		jsonParts = append(jsonParts, "}")
		jsonData := strings.Join(jsonParts, "")

		// 创建回调选项
		var wg sync.WaitGroup
		var mu sync.Mutex
		results := make(map[string]string)
		callbacks := make([]CallbackOption, 0, numFields)

		for i := 0; i < numFields; i++ {
			fieldName := fmt.Sprintf("field%d", i)
			wg.Add(1)
			callbacks = append(callbacks, WithRegisterFieldStreamHandler(fieldName, func(key string, reader io.Reader, parents []string) {
				defer wg.Done()
				data, err := io.ReadAll(reader)
				require.NoError(t, err)

				mu.Lock()
				results[fieldName] = string(data)
				mu.Unlock()
			}))
		}

		// 执行解析
		err := ExtractStructuredJSON(jsonData, callbacks...)
		require.NoError(t, err)

		// 等待所有处理完成
		done := make(chan bool)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-done:
			// 验证结果
			mu.Lock()
			defer mu.Unlock()

			assert.Equal(t, numFields, len(results), "All fields should be processed")
			for fieldName, expectedValue := range fieldData {
				receivedValue := results[fieldName]
				assert.Equal(t, fmt.Sprintf(`"%s"`, expectedValue), receivedValue,
					"Field %s should have correct value", fieldName)
			}

		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for concurrent processing")
		}
	})
}

func TestWithRegisterFieldStreamHandler_StreamInterruption(t *testing.T) {
	t.Run("Reader Closes Prematurely", func(t *testing.T) {
		// 创建一个会提前关闭的reader
		pr, pw := io.Pipe()

		go func() {
			// 写入部分数据后关闭
			pw.Write([]byte(`{"field": "partial`))
			time.Sleep(100 * time.Millisecond)
			pw.Close() // 提前关闭
		}()

		handlerCalled := false
		var handlerError error

		err := ExtractStructuredJSONFromStream(pr, WithRegisterFieldStreamHandler("field", func(key string, reader io.Reader, parents []string) {
			handlerCalled = true
			// 尝试读取所有数据
			_, handlerError = io.ReadAll(reader)
		}))

		// 解析可能会失败，但不应该panic
		t.Logf("Parse error: %v", err)
		t.Logf("Handler called: %v", handlerCalled)
		t.Logf("Handler error: %v", handlerError)
	})

	t.Run("Slow Reader", func(t *testing.T) {
		// 模拟慢速数据源
		pr, pw := io.Pipe()

		go func() {
			defer pw.Close()
			data := `{"slowfield": "` + strings.Repeat("slow data ", 100) + `"}`

			// 逐字节慢速写入
			for _, b := range []byte(data) {
				pw.Write([]byte{b})
				time.Sleep(100 * time.Microsecond) // 减少延迟，保持在1-2秒内
			}
		}()

		var receivedSize int
		handlerCalled := false

		err := ExtractStructuredJSONFromStream(pr, WithRegisterFieldStreamHandler("slowfield", func(key string, reader io.Reader, parents []string) {
			handlerCalled = true
			buffer := make([]byte, 100)

			for {
				n, err := reader.Read(buffer)
				if n > 0 {
					receivedSize += n
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Logf("Read error: %v", err)
					break
				}
			}
		}))

		require.NoError(t, err)
		assert.True(t, handlerCalled, "Handler should be called")
		assert.Greater(t, receivedSize, 1000, "Should receive substantial data")
	})
}

func TestWithRegisterFieldStreamHandler_MemoryStress(t *testing.T) {
	t.Run("Very Large Field", func(t *testing.T) {
		// 创建一个非常大的字段
		largeData := strings.Repeat("X", 1024*1024) // 1MB
		jsonData := fmt.Sprintf(`{"huge": "%s"}`, largeData)

		var totalReceived int
		var chunkCount int

		err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler("huge", func(key string, reader io.Reader, parents []string) {
			buffer := make([]byte, 4096) // 4KB缓冲区

			for {
				n, err := reader.Read(buffer)
				if n > 0 {
					totalReceived += n
					chunkCount++
				}
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
		}))

		require.NoError(t, err)
		// 总接收量应该包括引号
		assert.Equal(t, len(largeData)+2, totalReceived, "Should receive all data including quotes")
		assert.Greater(t, chunkCount, 1, "Should receive data in multiple chunks")

		t.Logf("Processed %d bytes in %d chunks", totalReceived, chunkCount)
	})
}

func TestWithRegisterFieldStreamHandler_ErrorHandling(t *testing.T) {
	t.Run("Handler Panic Recovery", func(t *testing.T) {
		// 测试handler中的panic是否会被正确处理
		jsonData := `{"panic_field": "trigger panic"}`

		err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler("panic_field", func(key string, reader io.Reader, parents []string) {
			panic("intentional panic for testing")
		}))

		// 主解析过程不应该因为handler的panic而崩溃
		require.NoError(t, err)
	})

	t.Run("Multiple Field Handlers with Some Failing", func(t *testing.T) {
		jsonData := `{
			"good1": "normal data 1",
			"bad": "trigger error",
			"good2": "normal data 2"
		}`

		var good1Data, good2Data string
		var good1Called, good2Called, badCalled bool

		err := ExtractStructuredJSON(jsonData,
			WithRegisterFieldStreamHandler("good1", func(key string, reader io.Reader, parents []string) {
				good1Called = true
				data, _ := io.ReadAll(reader)
				good1Data = string(data)
			}),
			WithRegisterFieldStreamHandler("bad", func(key string, reader io.Reader, parents []string) {
				badCalled = true
				panic("handler error")
			}),
			WithRegisterFieldStreamHandler("good2", func(key string, reader io.Reader, parents []string) {
				good2Called = true
				data, _ := io.ReadAll(reader)
				good2Data = string(data)
			}),
		)

		require.NoError(t, err)

		// 等待所有处理器完成
		timeout := time.After(1 * time.Second)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				t.Fatal("Timeout waiting for handlers to complete")
			case <-ticker.C:
				if good1Called && good2Called {
					goto done
				}
			}
		}
	done:

		assert.True(t, good1Called, "Good1 handler should be called")
		assert.True(t, badCalled, "Bad handler should be called")
		assert.True(t, good2Called, "Good2 handler should be called")
		assert.Equal(t, `"normal data 1"`, good1Data)
		assert.Equal(t, `"normal data 2"`, good2Data)
	})
}

func TestWithRegisterFieldStreamHandler_CombinedWithOtherCallbacks(t *testing.T) {
	t.Run("Mixed Callback Types", func(t *testing.T) {
		jsonData := `{
			"stream_field": "streaming data", 
			"regular_field": "regular data",
			"nested": {
				"inner": "value"
			},
			"array": [1, 2, 3]
		}`

		var streamData string
		var objects []map[string]any
		var arrays [][]any
		var rawKVs []struct{ key, value any }

		var streamCalled, objectCalled, arrayCalled, kvCalled atomic.Bool

		// 创建带缓冲的channel来收集信号
		streamDone := make(chan struct{}, 1)
		objectDone := make(chan struct{}, 1)
		arrayDone := make(chan struct{}, 1)
		kvDone := make(chan struct{}, 1)

		err := ExtractStructuredJSON(jsonData,
			WithRegisterFieldStreamHandler("stream_field", func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				streamCalled.Store(true)
				streamData = string(data)
				select {
				case streamDone <- struct{}{}:
				default:
				}
			}),
			WithObjectCallback(func(data map[string]any) {
				objectCalled.Store(true)
				objects = append(objects, data)
				select {
				case objectDone <- struct{}{}:
				default:
				}
			}),
			WithArrayCallback(func(data []any) {
				arrayCalled.Store(true)
				arrays = append(arrays, data)
				select {
				case arrayDone <- struct{}{}:
				default:
				}
			}),
			WithRawKeyValueCallback(func(key, data any) {
				kvCalled.Store(true)
				rawKVs = append(rawKVs, struct{ key, value any }{key, data})
				select {
				case kvDone <- struct{}{}:
				default:
				}
			}),
		)

		require.NoError(t, err)

		// 等待关键回调完成，设置超时
		timeout := time.After(1 * time.Second)

		select {
		case <-streamDone:
		case <-timeout:
			t.Fatal("Stream handler timeout")
		}

		select {
		case <-objectDone:
		case <-timeout:
			t.Fatal("Object callback timeout")
		}

		select {
		case <-arrayDone:
		case <-timeout:
			t.Fatal("Array callback timeout")
		}

		select {
		case <-kvDone:
		case <-timeout:
			t.Fatal("KV callback timeout")
		}

		assert.True(t, streamCalled.Load(), "Stream handler should be called")
		assert.True(t, objectCalled.Load(), "Object callback should be called")
		assert.True(t, arrayCalled.Load(), "Array callback should be called")
		assert.True(t, kvCalled.Load(), "KV callback should be called")

		assert.Equal(t, `"streaming data"`, streamData)
		assert.Greater(t, len(objects), 0)
		assert.Greater(t, len(arrays), 0)
		assert.Greater(t, len(rawKVs), 0)
	})
}
