package jsonextractor

import (
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

		err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler("key4", func(key string, reader io.Reader, parents []string) {
			receivedKey = key
			data, _ := io.ReadAll(reader)
			receivedData = string(data)
			receivedParents = make([]string, len(parents))
			copy(receivedParents, parents)
		}))

		require.NoError(t, err)
		assert.Equal(t, "key4", receivedKey)
		assert.Equal(t, `"simple value"`, receivedData)
		assert.Empty(t, receivedParents) // key4在根级别，没有父路径
	})

	t.Run("嵌套字段匹配", func(t *testing.T) {
		var receivedKey string
		var receivedData string
		var receivedParents []string

		err := ExtractStructuredJSON(jsonData, WithRegisterFieldStreamHandler("key3", func(key string, reader io.Reader, parents []string) {
			receivedKey = key
			data, _ := io.ReadAll(reader)
			receivedData = string(data)
			receivedParents = make([]string, len(parents))
			copy(receivedParents, parents)
		}))

		require.NoError(t, err)
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
	type result struct {
		key     string
		data    string
		parents []string
	}
	var results []result

	err := ExtractStructuredJSON(jsonData,
		WithRegisterFieldStreamHandler("target", func(key string, reader io.Reader, parents []string) {
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

	// 等待一下确保所有处理完成
	time.Sleep(100 * time.Millisecond)

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

	err := ExtractStructuredJSON(jsonData,
		WithRegisterFieldStreamHandler("large_field", func(key string, reader io.Reader, parents []string) {
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
	// 总接收量应该包括引号
	expectedSize := len(largeData) + 2
	assert.Equal(t, expectedSize, receivedSize)
	assert.Greater(t, chunkCount, 1, "Should receive data in multiple chunks")

	t.Logf("Processed %d bytes in %d chunks", receivedSize, chunkCount)
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
