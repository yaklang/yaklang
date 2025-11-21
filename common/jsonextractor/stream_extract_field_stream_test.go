package jsonextractor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试统一的字段流处理器API
func TestFieldStreamHandler_UnifiedAPI(t *testing.T) {
	jsonData := `{
		"key1": {
			"key2": [
				{"key3": "abc123"}
			]
		},
		"key4": "simple value"
	}`

	t.Run("基础字段匹配", func(t *testing.T) {
		var receivedKey string
		var receivedData string
		var receivedParents []string
		var wg sync.WaitGroup
		wg.Add(1)

		err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler("key4", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			receivedKey = key
			data, _ := io.ReadAll(reader)
			receivedData = string(data)
			receivedParents = make([]string, len(parents))
			copy(receivedParents, parents)
		}))

		require.NoError(t, err)
		wg.Wait()
		assert.Equal(t, "key4", receivedKey)
		assert.Equal(t, `"simple value"`, receivedData)
		assert.Empty(t, receivedParents) // key4在根级别，没有父路径
	})

	t.Run("嵌套字段匹配", func(t *testing.T) {
		var receivedKey string
		var receivedData string
		var receivedParents []string
		var wg sync.WaitGroup
		wg.Add(1)

		err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler("key3", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			receivedKey = key
			data, _ := io.ReadAll(reader)
			receivedData = string(data)
			receivedParents = make([]string, len(parents))
			copy(receivedParents, parents)
		}))

		require.NoError(t, err)
		wg.Wait()
		assert.Equal(t, "key3", receivedKey)
		assert.Equal(t, `"abc123"`, receivedData)
		// key3的父路径应该是: key1 -> key2 -> [0]
		t.Logf("Parents: %v", receivedParents)
		assert.Contains(t, receivedParents, "key1")
		assert.Contains(t, receivedParents, "key2")
	})
}

func TestFieldStreamHandler_MultipleFields(t *testing.T) {
	jsonData := `{
		"field1": "data1",
		"field2": "data2",
		"field3": "data3",
		"other": "ignored"
	}`

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make(map[string]string)

	wg.Add(3)

	err := ExtractStructuredJSON(jsonData,
		WithRegisterMultiFieldStreamHandler([]string{"field1", "field2", "field3"}, func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, _ := io.ReadAll(reader)
			mu.Lock()
			results[key] = string(data)
			mu.Unlock()
		}))

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
		assert.Equal(t, 3, len(results))
		assert.Equal(t, `"data1"`, results["field1"])
		assert.Equal(t, `"data2"`, results["field2"])
		assert.Equal(t, `"data3"`, results["field3"])
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for processing")
	}
}

func TestFieldStreamHandler_RegexpMatching(t *testing.T) {
	jsonData := `{
		"user_name": "alice",
		"user_age": "25",
		"admin_role": "admin",
		"user_email": "alice@example.com",
		"other_field": "ignored"
	}`

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make(map[string]string)

	// 匹配所有以"user_"开头的字段
	wg.Add(3) // user_name, user_age, user_email

	err := ExtractStructuredJSON(jsonData,
		WithRegisterRegexpFieldStreamHandler("^user_.*", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, _ := io.ReadAll(reader)
			mu.Lock()
			results[key] = string(data)
			mu.Unlock()
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
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, 3, len(results))
		assert.Equal(t, `"alice"`, results["user_name"])
		assert.Equal(t, `"25"`, results["user_age"])
		assert.Equal(t, `"alice@example.com"`, results["user_email"])
		// admin_role 和 other_field 应该被忽略
		assert.NotContains(t, results, "admin_role")
		assert.NotContains(t, results, "other_field")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for processing")
	}
}

func TestFieldStreamHandler_GlobMatching(t *testing.T) {
	jsonData := `{
		"config_database": "mysql",
		"config_cache": "redis",
		"setting_theme": "dark",
		"config_port": "3306",
		"other": "ignored"
	}`

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make(map[string]string)

	// 匹配所有以"config_"开头的字段
	wg.Add(3) // config_database, config_cache, config_port

	err := ExtractStructuredJSON(jsonData,
		WithRegisterGlobFieldStreamHandler("config_*", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, _ := io.ReadAll(reader)
			mu.Lock()
			results[key] = string(data)
			mu.Unlock()
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
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, 3, len(results))
		assert.Equal(t, `"mysql"`, results["config_database"])
		assert.Equal(t, `"redis"`, results["config_cache"])
		assert.Equal(t, `"3306"`, results["config_port"])
		// setting_theme 和 other 应该被忽略
		assert.NotContains(t, results, "setting_theme")
		assert.NotContains(t, results, "other")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for processing")
	}
}

func TestFieldStreamHandler_ComplexNestingWithParents(t *testing.T) {
	jsonData := `{
		"level1": {
			"level2": {
				"level3": {
					"target": "found it!"
				},
				"array": [
					{"target": "in array"}
				]
			}
		},
		"root_target": "at root"
	}`

	var mu sync.Mutex
	var wg sync.WaitGroup
	type result struct {
		key     string
		data    string
		parents []string
	}
	var results []result

	// 预期有2个 "target" 字段：deep nested target 和 array target （root_target 是不同的key）
	wg.Add(2)

	err := ExtractStructuredJSON(jsonData,
		WithRegisterFieldStreamHandler("target", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, _ := io.ReadAll(reader)
			mu.Lock()
			parentsCopy := make([]string, len(parents))
			copy(parentsCopy, parents)
			results = append(results, result{
				key:     key,
				data:    string(data),
				parents: parentsCopy,
			})
			mu.Unlock()
		}))

	require.NoError(t, err)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, 2, len(results))

	// 查找深层嵌套的target
	var deepResult *result
	var arrayResult *result
	for i := range results {
		if strings.Contains(results[i].data, "found it!") {
			deepResult = &results[i]
		} else if strings.Contains(results[i].data, "in array") {
			arrayResult = &results[i]
		}
	}

	require.NotNil(t, deepResult, "Should find deeply nested target")
	assert.Equal(t, `"found it!"`, deepResult.data)
	assert.Contains(t, deepResult.parents, "level1")
	assert.Contains(t, deepResult.parents, "level2")
	assert.Contains(t, deepResult.parents, "level3")

	require.NotNil(t, arrayResult, "Should find array target")
	assert.Equal(t, `"in array"`, arrayResult.data)
	assert.Contains(t, arrayResult.parents, "level1")
	assert.Contains(t, arrayResult.parents, "level2")
	assert.Contains(t, arrayResult.parents, "array")
}

func TestFieldStreamHandler_LargeDataStreaming(t *testing.T) {
	// 创建大字段数据
	largeData := strings.Repeat("Large content data. ", 10000) // 约200KB
	jsonData := fmt.Sprintf(`{"large_field": "%s"}`, largeData)

	var receivedSize int
	var chunkCount int
	var wg sync.WaitGroup
	wg.Add(1)

	err := ExtractStructuredJSON(jsonData,
		WithRegisterFieldStreamHandler("large_field", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			buffer := make([]byte, 4096) // 4KB缓冲区

			for {
				n, err := reader.Read(buffer)
				if n > 0 {
					receivedSize += n
					chunkCount++
				}
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
		}))

	require.NoError(t, err)
	wg.Wait()
	// 总接收量应该包括引号
	expectedSize := len(largeData) + 2
	assert.Equal(t, expectedSize, receivedSize)
	assert.Greater(t, chunkCount, 1, "Should receive data in multiple chunks")

	t.Logf("Processed %d bytes in %d chunks", receivedSize, chunkCount)
}

func TestFieldStreamHandler_StreamArrayValueKeepsStructure(t *testing.T) {
	jsonData := `{"key": ["1", "2", "3"]}`
	var wg sync.WaitGroup
	var received string

	wg.Add(1)
	err := ExtractStructuredJSON(jsonData,
		WithRegisterFieldStreamHandler("key", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			received = string(data)
		}))

	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		var arr []string
		require.NoError(t, json.Unmarshal([]byte(received), &arr))
		assert.Equal(t, []string{"1", "2", "3"}, arr)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for array stream callback")
	}
}

func normalizeJSONForAssert(s string) string {
	replacer := strings.NewReplacer(
		" ", "",
		"\n", "",
		"\t", "",
		"\r", "",
	)
	return replacer.Replace(s)
}

func TestFieldStreamHandler_ComplexCompositePayload(t *testing.T) {
	jsonData := `{"@action": "aaa", "arr": ["123123", {"arr2": [1, 2, 3, ["3333"]]}, "aaa"]}`
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[string]string)

	expectedKeys := map[string]struct{}{
		"@action": {},
		"arr":     {},
	}

	wg.Add(len(expectedKeys))

	err := ExtractStructuredJSON(jsonData,
		WithRegisterMultiFieldStreamHandler([]string{"@action", "arr"}, func(key string, reader io.Reader, parents []string) {
			if _, ok := expectedKeys[key]; !ok {
				return
			}
			data, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			mu.Lock()
			results[key] = string(data)
			mu.Unlock()
			wg.Done()
		}))

	require.NoError(t, err)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, len(expectedKeys), len(results))

	assert.Equal(t, `"aaa"`, results["@action"])

	arrNormalized := normalizeJSONForAssert(results["arr"])
	assert.Contains(t, arrNormalized, `"123123"`)
	assert.Contains(t, arrNormalized, `"arr2":[1,2,3,["3333"]]`)
	assert.Contains(t, arrNormalized, `"aaa"`)
}

func TestFieldStreamHandler_NestedCompositeSameKey(t *testing.T) {
	jsonData := `{"key": {"key": 2}}`
	var wg sync.WaitGroup
	var mu sync.Mutex
	type record struct {
		data    string
		parents []string
	}
	var records []record

	wg.Add(2)
	err := ExtractStructuredJSON(jsonData,
		WithRegisterFieldStreamHandler("key", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			parentCopy := make([]string, len(parents))
			copy(parentCopy, parents)
			records = append(records, record{
				data:    string(data),
				parents: parentCopy,
			})
		}))

	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Len(t, records, 2)
		var outer, inner *record
		for i := range records {
			if len(records[i].parents) == 0 {
				outer = &records[i]
			} else {
				inner = &records[i]
			}
		}
		require.NotNil(t, outer, "outer field stream should exist")
		require.NotNil(t, inner, "inner field stream should exist")
		assert.Equal(t, `{"key": 2}`, outer.data)
		assert.Equal(t, `2`, inner.data)
		assert.Contains(t, inner.parents, "key")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for nested stream callbacks")
	}
}

func TestFieldStreamHandler_BoundaryCompositeVariants(t *testing.T) {
	jsonData := `{
		"empty_object": {},
		"empty_array": [],
		"nested_object_array": {
			"outer": [
				{"inner": []},
				[{"deep": "value"}]
			]
		},
		"array_of_objects": [
			{"id": 1},
			{"id": 2}
		],
		"array_with_primitives": [true, false, null, 0]
	}`

	expectedKeys := map[string]struct{}{
		"empty_object":          {},
		"empty_array":           {},
		"nested_object_array":   {},
		"array_of_objects":      {},
		"array_with_primitives": {},
	}

	var wg sync.WaitGroup
	wg.Add(len(expectedKeys))

	var mu sync.Mutex
	results := make(map[string]string, len(expectedKeys))

	err := ExtractStructuredJSON(jsonData,
		WithRegisterMultiFieldStreamHandler(
			[]string{
				"empty_object",
				"empty_array",
				"nested_object_array",
				"array_of_objects",
				"array_with_primitives",
			},
			func(key string, reader io.Reader, parents []string) {
				if _, ok := expectedKeys[key]; !ok {
					return
				}

				data, readErr := io.ReadAll(reader)
				require.NoError(t, readErr)
				mu.Lock()
				results[key] = string(data)
				mu.Unlock()
				wg.Done()
			}))

	require.NoError(t, err)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, len(expectedKeys), len(results))

	normalized := make(map[string]string, len(results))
	for k, v := range results {
		normalized[k] = normalizeJSONForAssert(v)
	}

	assert.Equal(t, `{}`, normalized["empty_object"])
	assert.Equal(t, `[]`, normalized["empty_array"])
	assert.Contains(t, normalized["nested_object_array"], `"outer":[{"inner":[]},[{"deep":"value"}]]`)
	assert.Contains(t, normalized["array_of_objects"], `"id":1`)
	assert.Contains(t, normalized["array_of_objects"], `"id":2`)
	assert.Contains(t, normalized["array_with_primitives"], `true`)
	assert.Contains(t, normalized["array_with_primitives"], `false`)
	assert.Contains(t, normalized["array_with_primitives"], `null`)
	assert.Contains(t, normalized["array_with_primitives"], `0`)
}

func TestFieldStreamHandler_CombinedMatchers(t *testing.T) {
	jsonData := `{
		"user_name": "alice",
		"config_db": "mysql", 
		"temp_file": "temp.txt",
		"debug_log": "debug info",
		"user_email": "alice@example.com"
	}`

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make(map[string]map[string]string)

	// 初始化结果map
	results["user"] = make(map[string]string)
	results["config"] = make(map[string]string)
	results["temp"] = make(map[string]string)

	wg.Add(5) // 预期5个字段会被匹配

	err := ExtractStructuredJSON(jsonData,
		// 使用正则匹配user_开头的字段
		WithRegisterRegexpFieldStreamHandler("^user_.*", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, _ := io.ReadAll(reader)
			mu.Lock()
			results["user"][key] = string(data)
			mu.Unlock()
		}),
		// 使用glob匹配config_开头的字段
		WithRegisterGlobFieldStreamHandler("config_*", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, _ := io.ReadAll(reader)
			mu.Lock()
			results["config"][key] = string(data)
			mu.Unlock()
		}),
		// 使用多字段匹配
		WithRegisterMultiFieldStreamHandler([]string{"temp_file", "debug_log"}, func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, _ := io.ReadAll(reader)
			mu.Lock()
			results["temp"][key] = string(data)
			mu.Unlock()
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
		mu.Lock()
		defer mu.Unlock()

		// 验证user字段
		assert.Equal(t, 2, len(results["user"]))
		assert.Equal(t, `"alice"`, results["user"]["user_name"])
		assert.Equal(t, `"alice@example.com"`, results["user"]["user_email"])

		// 验证config字段
		assert.Equal(t, 1, len(results["config"]))
		assert.Equal(t, `"mysql"`, results["config"]["config_db"])

		// 验证temp字段
		assert.Equal(t, 2, len(results["temp"]))
		assert.Equal(t, `"temp.txt"`, results["temp"]["temp_file"])
		assert.Equal(t, `"debug info"`, results["temp"]["debug_log"])

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for processing")
	}
}

func TestFieldStreamHandler_ErrorHandling(t *testing.T) {
	jsonData := `{"test_field": "test data"}`

	t.Run("Handler Panic Recovery", func(t *testing.T) {
		err := ExtractStructuredJSON(jsonData,
			WithRegisterFieldStreamHandler("test_field", func(key string, reader io.Reader, parents []string) {
				panic("intentional panic for testing")
			}))

		// 主解析过程不应该因为handler的panic而崩溃
		require.NoError(t, err)
	})

	t.Run("Invalid Pattern Handling", func(t *testing.T) {
		// 测试无效的正则表达式
		err := ExtractStructuredJSON(jsonData,
			WithRegisterRegexpFieldStreamHandler("[invalid", func(key string, reader io.Reader, parents []string) {
				t.Log("This should not be called")
			}))

		// 应该不会崩溃，只是模式不匹配
		require.NoError(t, err)
	})
}

func TestFieldStreamHandler_FromStream(t *testing.T) {
	jsonData := `{
		"stream_field1": "streaming data 1",
		"stream_field2": "streaming data 2"
	}`

	reader := strings.NewReader(jsonData)

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make(map[string]string)

	wg.Add(2)

	err := ExtractStructuredJSONFromStream(reader,
		WithRegisterGlobFieldStreamHandler("stream_*", func(key string, reader io.Reader, parents []string) {
			defer wg.Done()
			data, _ := io.ReadAll(reader)
			mu.Lock()
			results[key] = string(data)
			mu.Unlock()
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
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, 2, len(results))
		assert.Equal(t, `"streaming data 1"`, results["stream_field1"])
		assert.Equal(t, `"streaming data 2"`, results["stream_field2"])
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for stream processing")
	}
}

func TestFieldStreamHandler_StreamedReaderInput(t *testing.T) {
	pr, pw := io.Pipe()
	firstChunkRead := make(chan struct{})
	handlerDone := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- ExtractStructuredJSONFromStream(pr,
			WithRegisterFieldStreamHandler("payload", func(key string, reader io.Reader, parents []string) {
				var notified sync.Once
				var buf bytes.Buffer
				tmp := make([]byte, 4)
				for {
					n, err := reader.Read(tmp)
					if n > 0 {
						buf.Write(tmp[:n])
						notified.Do(func() {
							close(firstChunkRead)
						})
					}
					if err == io.EOF {
						break
					}
					require.NoError(t, err)
				}
				assert.Equal(t, `"hello streaming"`, buf.String())
				close(handlerDone)
			}))
	}()

	_, err := pw.Write([]byte(`{"payload":"hel`))
	require.NoError(t, err)

	select {
	case <-firstChunkRead:
	case <-time.After(time.Second):
		t.Fatal("field stream reader did not receive partial data in time")
	}

	_, err = pw.Write([]byte(`lo streaming"}`))
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	require.NoError(t, <-errCh)
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish reading stream data")
	}
}

func TestFieldStreamHandler_StreamClosesOnInputError(t *testing.T) {
	pr, pw := io.Pipe()
	handlerDone := make(chan struct{})
	errCh := make(chan error, 1)
	closeErr := errors.New("upstream boom")

	go func() {
		errCh <- ExtractStructuredJSONFromStream(pr,
			WithRegisterFieldStreamHandler("payload", func(key string, reader io.Reader, parents []string) {
				data, err := io.ReadAll(reader)
				require.NoError(t, err)
				assert.Equal(t, `"partial`, string(data))
				close(handlerDone)
			}))
	}()

	_, err := pw.Write([]byte(`{"payload":"partial`))
	require.NoError(t, err)
	require.NoError(t, pw.CloseWithError(closeErr))

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.ErrorIs(t, err, closeErr)
	case <-time.After(time.Second):
		t.Fatal("extractor did not return after upstream error")
	}

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("field stream reader was not closed after upstream error")
	}
}

func TestFieldStreamHandler_StreamedCompositeValues(t *testing.T) {
	pr, pw := io.Pipe()
	handlerDone := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- ExtractStructuredJSONFromStream(pr,
			WithRegisterFieldStreamHandler("config", func(key string, reader io.Reader, parents []string) {
				var buf bytes.Buffer
				tmp := make([]byte, 8)
				for {
					n, err := reader.Read(tmp)
					if n > 0 {
						buf.Write(tmp[:n])
					}
					if err == io.EOF {
						break
					}
					require.NoError(t, err)
				}
				normalized := normalizeJSONForAssert(buf.String())
				assert.Equal(t, `{"db":{"hosts":["10.0.0.1"],"port":5432},"features":[true,false,null]}`, normalized)
				close(handlerDone)
			}))
	}()

	chunks := []string{
		`{"config":{"db":{"hosts":["10.0.`,
		`0.1"],"port":5432},"features":[true,`,
		`false,null]}}`,
	}

	for _, chunk := range chunks {
		_, err := pw.Write([]byte(chunk))
		require.NoError(t, err)
		time.Sleep(5 * time.Millisecond)
	}
	require.NoError(t, pw.Close())

	require.NoError(t, <-errCh)
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish reading composite stream data")
	}
}

func TestFieldStreamHandler_MultipleHandlersReceiveSameStream(t *testing.T) {
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	var mu sync.Mutex
	results := make(map[string]string)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		errCh <- ExtractStructuredJSONFromStream(pr,
			WithRegisterFieldStreamHandler("payload", func(key string, reader io.Reader, parents []string) {
				defer wg.Done()
				data, err := io.ReadAll(reader)
				require.NoError(t, err)
				mu.Lock()
				results["exact"] = string(data)
				mu.Unlock()
			}),
			WithRegisterRegexpFieldStreamHandler("^payload$", func(key string, reader io.Reader, parents []string) {
				defer wg.Done()
				data, err := io.ReadAll(reader)
				require.NoError(t, err)
				mu.Lock()
				results["regex"] = string(data)
				mu.Unlock()
			}),
		)
	}()

	parts := []string{`{"payload":"mir`, `rored stream"}`}
	for _, part := range parts {
		_, err := pw.Write([]byte(part))
		require.NoError(t, err)
	}
	require.NoError(t, pw.Close())

	require.NoError(t, <-errCh)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("field stream handlers did not finish")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, `"mirrored stream"`, results["exact"])
	assert.Equal(t, `"mirrored stream"`, results["regex"])
}

func TestFieldStreamHandler_StartCallbackFiresBeforeValue(t *testing.T) {
	pr, pw := io.Pipe()
	startCalled := make(chan struct{}, 1)
	handlerDone := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- ExtractStructuredJSONFromStream(pr,
			WithRegisterFieldStreamHandlerAndStartCallback(
				"payload",
				func(key string, reader io.Reader, parents []string) {
					data, err := io.ReadAll(reader)
					require.NoError(t, err)
					assert.Equal(t, `"gate"`, string(data))
					close(handlerDone)
				},
				func(key string, reader io.Reader, parents []string) {
					select {
					case startCalled <- struct{}{}:
					default:
					}
				},
			))
	}()

	_, err := pw.Write([]byte(`{"payload":`))
	require.NoError(t, err)

	select {
	case <-startCalled:
	case <-time.After(time.Second):
		t.Fatal("start callback was not invoked before payload streaming")
	}

	_, err = pw.Write([]byte(`"gate"}`))
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	require.NoError(t, <-errCh)
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after start callback")
	}
}
