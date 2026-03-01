package jsonextractor

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// simulates real-world planTasksStreamHandler pattern:
// ExtractStructuredJSONFromStream reads an array of objects,
// each handler goroutine formats output and writes to a shared buffer.
// The core fix ensures ALL goroutines complete before the function returns.

func TestFieldStreamGoroutineSync_ArrayOfObjectsAllFieldsPresent(t *testing.T) {
	input := `[
		{"name":"task_a","goal":"Goal for A"},
		{"name":"task_b","goal":"Goal for B"},
		{"name":"task_c","goal":"Goal for C"}
	]`

	var mu sync.Mutex
	results := make(map[string][]string)

	err := ExtractStructuredJSON(input,
		WithRegisterMultiFieldStreamHandler(
			[]string{"name", "goal"},
			func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				val := strings.Trim(string(data), `"`)
				mu.Lock()
				results[key] = append(results[key], val)
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, results["name"], 3, "expected 3 names, got: %v", results["name"])
	require.Len(t, results["goal"], 3, "expected 3 goals, got: %v", results["goal"])
	assert.Contains(t, results["name"], "task_a")
	assert.Contains(t, results["name"], "task_b")
	assert.Contains(t, results["name"], "task_c")
	assert.Contains(t, results["goal"], "Goal for A")
	assert.Contains(t, results["goal"], "Goal for B")
	assert.Contains(t, results["goal"], "Goal for C")
}

func TestFieldStreamGoroutineSync_ArrayOfObjectsWithOptionalFields(t *testing.T) {
	input := `[
		{"name":"setup","id":"setup_env","goal":"Setup env","deps":[]},
		{"name":"build","id":"build_proj","goal":"Build project","deps":["setup"]},
		{"name":"test","goal":"Run tests"}
	]`

	var mu sync.Mutex
	var buf bytes.Buffer
	var count atomic.Int32

	err := ExtractStructuredJSON(input,
		WithRegisterMultiFieldStreamHandler(
			[]string{"name", "goal", "id", "deps"},
			func(key string, reader io.Reader, parents []string) {
				raw, _ := io.ReadAll(reader)
				val := strings.Trim(string(raw), `"`)
				mu.Lock()
				switch key {
				case "name":
					if count.Add(1) > 1 {
						buf.WriteString("\n")
					}
					buf.WriteString("[" + val + "]")
				case "id":
					buf.WriteString(" #" + val)
				case "goal":
					buf.WriteString(": " + val)
				case "deps":
					trimmed := strings.TrimSpace(val)
					if trimmed != "" && trimmed != "[]" {
						buf.WriteString(" (deps:" + trimmed + ")")
					}
				}
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[setup]")
	assert.Contains(t, output, "#setup_env")
	assert.Contains(t, output, ": Setup env")
	assert.Contains(t, output, "[build]")
	assert.Contains(t, output, "#build_proj")
	assert.Contains(t, output, ": Build project")
	assert.Contains(t, output, "[test]")
	assert.Contains(t, output, ": Run tests")
}

func TestFieldStreamGoroutineSync_PipeStreamedInput(t *testing.T) {
	input := `[{"name":"first","goal":"First goal"},{"name":"second","goal":"Second goal"},{"name":"third","goal":"Third goal"}]`

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < len(input); i += 20 {
			end := i + 20
			if end > len(input) {
				end = len(input)
			}
			pw.Write([]byte(input[i:end]))
			time.Sleep(time.Millisecond)
		}
	}()

	var mu sync.Mutex
	results := make(map[string][]string)

	err := ExtractStructuredJSONFromStream(pr,
		WithRegisterMultiFieldStreamHandler(
			[]string{"name", "goal"},
			func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				val := strings.Trim(string(data), `"`)
				mu.Lock()
				results[key] = append(results[key], val)
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, results["name"], 3)
	require.Len(t, results["goal"], 3)
	assert.Contains(t, results["name"], "first")
	assert.Contains(t, results["name"], "second")
	assert.Contains(t, results["name"], "third")
	assert.Contains(t, results["goal"], "First goal")
	assert.Contains(t, results["goal"], "Second goal")
	assert.Contains(t, results["goal"], "Third goal")
}

func TestFieldStreamGoroutineSync_ByteByByteStreaming(t *testing.T) {
	input := `[{"k":"v1"},{"k":"v2"},{"k":"v3"}]`

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < len(input); i++ {
			pw.Write([]byte{input[i]})
			time.Sleep(500 * time.Microsecond)
		}
	}()

	var mu sync.Mutex
	var results []string

	err := ExtractStructuredJSONFromStream(pr,
		WithRegisterFieldStreamHandler("k", func(key string, reader io.Reader, parents []string) {
			data, _ := io.ReadAll(reader)
			mu.Lock()
			results = append(results, string(data))
			mu.Unlock()
		}),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, results, 3)
	assert.Equal(t, `"v1"`, results[0])
	assert.Equal(t, `"v2"`, results[1])
	assert.Equal(t, `"v3"`, results[2])
}

func TestFieldStreamGoroutineSync_ManyObjectsStressTest(t *testing.T) {
	var sb strings.Builder
	n := 100
	sb.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`{"idx":"%d","val":"data_%d"}`, i, i))
	}
	sb.WriteString("]")

	var mu sync.Mutex
	idxResults := make(map[string]string)

	err := ExtractStructuredJSON(sb.String(),
		WithRegisterMultiFieldStreamHandler(
			[]string{"idx", "val"},
			func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				val := strings.Trim(string(data), `"`)
				mu.Lock()
				if key == "val" {
					idxResults[val] = "seen"
				}
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("data_%d", i)
		assert.Equal(t, "seen", idxResults[key], "missing: %s", key)
	}
}

func TestFieldStreamGoroutineSync_MalformedJSON_NoHang(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"truncated_after_colon", `[{"name":"ok","goal":`},
		{"truncated_mid_key", `[{"name":"ok","go`},
		{"truncated_mid_value", `[{"name":"ok","goal":"partial`},
		{"truncated_after_comma", `[{"name":"ok"},`},
		{"truncated_array_open", `[{"name":"ok","arr":[`},
		{"truncated_nested_object", `[{"name":"ok","obj":{"inner":`},
		{"empty_truncated", `[{`},
		{"only_open_bracket", `[`},
		{"key_no_value", `[{"name":`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				defer close(done)
				ExtractStructuredJSON(tc.input,
					WithRegisterMultiFieldStreamHandler(
						[]string{"name", "goal", "arr", "obj"},
						func(key string, reader io.Reader, parents []string) {
							io.ReadAll(reader)
						},
					),
				)
			}()

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatalf("hung on malformed input: %q", tc.input)
			}
		})
	}
}

func TestFieldStreamGoroutineSync_MalformedJSON_PipeInput_NoHang(t *testing.T) {
	cases := []struct {
		name   string
		chunks []string
	}{
		{
			"pipe_truncated_mid_value",
			[]string{`[{"name":"ok","goal":"par`, `tial`},
		},
		{
			"pipe_truncated_after_first_object",
			[]string{`[{"name":"a","goal":"b"},`, `{"name":"c","goal`},
		},
		{
			"pipe_sudden_close",
			[]string{`[{"name":"x`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr, pw := io.Pipe()
			go func() {
				for _, chunk := range tc.chunks {
					pw.Write([]byte(chunk))
					time.Sleep(time.Millisecond)
				}
				pw.Close()
			}()

			done := make(chan struct{})
			go func() {
				defer close(done)
				ExtractStructuredJSONFromStream(pr,
					WithRegisterMultiFieldStreamHandler(
						[]string{"name", "goal"},
						func(key string, reader io.Reader, parents []string) {
							io.ReadAll(reader)
						},
					),
				)
			}()

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatalf("hung on pipe input: %v", tc.chunks)
			}
		})
	}
}

func TestFieldStreamGoroutineSync_PipeError_NoHang(t *testing.T) {
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte(`[{"name":"ok","goal":"start`))
		time.Sleep(10 * time.Millisecond)
		pw.CloseWithError(fmt.Errorf("simulated upstream error"))
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		ExtractStructuredJSONFromStream(pr,
			WithRegisterMultiFieldStreamHandler(
				[]string{"name", "goal"},
				func(key string, reader io.Reader, parents []string) {
					io.ReadAll(reader)
				},
			),
		)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("hung on pipe error")
	}
}

func TestFieldStreamGoroutineSync_HandlerPanic_NoLeak(t *testing.T) {
	input := `[{"name":"a","goal":"b"},{"name":"c","goal":"d"}]`

	var mu sync.Mutex
	var results []string

	done := make(chan struct{})
	go func() {
		defer close(done)
		ExtractStructuredJSON(input,
			WithRegisterMultiFieldStreamHandler(
				[]string{"name", "goal"},
				func(key string, reader io.Reader, parents []string) {
					data, _ := io.ReadAll(reader)
					if key == "name" && strings.Contains(string(data), "a") {
						panic("intentional test panic")
					}
					mu.Lock()
					results = append(results, key+"="+string(data))
					mu.Unlock()
				},
			),
		)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("hung after handler panic")
	}
}

func TestFieldStreamGoroutineSync_CompletionOrder(t *testing.T) {
	input := `[
		{"step":"1","desc":"first"},
		{"step":"2","desc":"second"},
		{"step":"3","desc":"third"},
		{"step":"4","desc":"fourth"},
		{"step":"5","desc":"fifth"}
	]`

	var mu sync.Mutex
	var completionLog []string

	err := ExtractStructuredJSON(input,
		WithRegisterMultiFieldStreamHandler(
			[]string{"step", "desc"},
			func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				val := strings.Trim(string(data), `"`)
				mu.Lock()
				completionLog = append(completionLog, key+":"+val)
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, completionLog, 10, "expected 10 entries (5 steps + 5 descs), got %d: %v", len(completionLog), completionLog)

	expectedEntries := map[string]bool{
		"step:1": false, "desc:first": false,
		"step:2": false, "desc:second": false,
		"step:3": false, "desc:third": false,
		"step:4": false, "desc:fourth": false,
		"step:5": false, "desc:fifth": false,
	}
	for _, entry := range completionLog {
		if _, ok := expectedEntries[entry]; ok {
			expectedEntries[entry] = true
		}
	}
	for entry, seen := range expectedEntries {
		assert.True(t, seen, "missing entry: %s", entry)
	}
}

func TestFieldStreamGoroutineSync_NestedObjectInArray(t *testing.T) {
	input := `[
		{"name":"task1","config":{"timeout":30,"retry":true}},
		{"name":"task2","config":{"timeout":60,"retry":false}}
	]`

	var mu sync.Mutex
	var names []string
	var configs []string

	err := ExtractStructuredJSON(input,
		WithRegisterMultiFieldStreamHandler(
			[]string{"name", "config"},
			func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				mu.Lock()
				switch key {
				case "name":
					names = append(names, strings.Trim(string(data), `"`))
				case "config":
					configs = append(configs, string(data))
				}
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, names, 2)
	require.Len(t, configs, 2)
	assert.Equal(t, "task1", names[0])
	assert.Equal(t, "task2", names[1])
	for _, cfg := range configs {
		assert.Contains(t, cfg, "timeout")
		assert.Contains(t, cfg, "retry")
	}
}

func TestFieldStreamGoroutineSync_UnicodeMultiObject(t *testing.T) {
	input := `[
		{"title":"配置开发环境","detail":"安装所有依赖工具"},
		{"title":"编写核心模块","detail":"实现业务逻辑"},
		{"title":"部署到生产环境","detail":"上线并监控"}
	]`

	var mu sync.Mutex
	results := make(map[string][]string)

	err := ExtractStructuredJSON(input,
		WithRegisterMultiFieldStreamHandler(
			[]string{"title", "detail"},
			func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				val := strings.Trim(string(data), `"`)
				mu.Lock()
				results[key] = append(results[key], val)
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, results["title"], 3, "expected 3 titles, got: %v", results["title"])
	require.Len(t, results["detail"], 3, "expected 3 details, got: %v", results["detail"])
	assert.Contains(t, results["title"], "配置开发环境")
	assert.Contains(t, results["title"], "编写核心模块")
	assert.Contains(t, results["title"], "部署到生产环境")
	assert.Contains(t, results["detail"], "安装所有依赖工具")
	assert.Contains(t, results["detail"], "实现业务逻辑")
	assert.Contains(t, results["detail"], "上线并监控")
}

func TestFieldStreamGoroutineSync_SlowPipeChunkedMultiObject(t *testing.T) {
	input := `[{"name":"alpha","goal":"Alpha goal"},{"name":"beta","goal":"Beta goal"},{"name":"gamma","goal":"Gamma goal"}]`

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		chunkSize := 7
		for i := 0; i < len(input); i += chunkSize {
			end := i + chunkSize
			if end > len(input) {
				end = len(input)
			}
			pw.Write([]byte(input[i:end]))
			time.Sleep(2 * time.Millisecond)
		}
	}()

	var mu sync.Mutex
	results := make(map[string][]string)

	err := ExtractStructuredJSONFromStream(pr,
		WithRegisterMultiFieldStreamHandler(
			[]string{"name", "goal"},
			func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				val := strings.Trim(string(data), `"`)
				mu.Lock()
				results[key] = append(results[key], val)
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, results["name"], 3)
	require.Len(t, results["goal"], 3)
	assert.Contains(t, results["name"], "alpha")
	assert.Contains(t, results["name"], "beta")
	assert.Contains(t, results["name"], "gamma")
	assert.Contains(t, results["goal"], "Alpha goal")
	assert.Contains(t, results["goal"], "Beta goal")
	assert.Contains(t, results["goal"], "Gamma goal")
}

func TestFieldStreamGoroutineSync_EmptyAndSingleElementArrays(t *testing.T) {
	t.Run("empty_array", func(t *testing.T) {
		var called atomic.Int32
		err := ExtractStructuredJSON(`[]`,
			WithRegisterFieldStreamHandler("any", func(key string, reader io.Reader, parents []string) {
				called.Add(1)
				io.ReadAll(reader)
			}),
		)
		require.NoError(t, err)
		assert.Equal(t, int32(0), called.Load())
	})

	t.Run("single_element", func(t *testing.T) {
		var mu sync.Mutex
		var result string
		err := ExtractStructuredJSON(`[{"only":"value"}]`,
			WithRegisterFieldStreamHandler("only", func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				mu.Lock()
				result = string(data)
				mu.Unlock()
			}),
		)
		require.NoError(t, err)
		assert.Equal(t, `"value"`, result)
	})
}

func TestFieldStreamGoroutineSync_FinishedCallbackAfterAllGoroutines(t *testing.T) {
	input := `[{"k":"v1"},{"k":"v2"},{"k":"v3"}]`

	var mu sync.Mutex
	var results []string
	var finishedAfterAll bool

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		pw.Write([]byte(input))
	}()

	err := ExtractStructuredJSONFromStream(pr,
		WithRegisterFieldStreamHandler("k", func(key string, reader io.Reader, parents []string) {
			data, _ := io.ReadAll(reader)
			mu.Lock()
			results = append(results, string(data))
			mu.Unlock()
		}),
		WithStreamFinishedCallback(func() {
			mu.Lock()
			finishedAfterAll = len(results) == 3
			mu.Unlock()
		}),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, results, 3)
	assert.True(t, finishedAfterAll, "stream finished callback should fire after all goroutines complete")
}

func TestFieldStreamGoroutineSync_RepeatRunsNeverFlaky(t *testing.T) {
	input := `[{"a":"1","b":"2"},{"a":"3","b":"4"},{"a":"5","b":"6"}]`

	for iter := 0; iter < 50; iter++ {
		var mu sync.Mutex
		var pairs []string

		err := ExtractStructuredJSON(input,
			WithRegisterMultiFieldStreamHandler(
				[]string{"a", "b"},
				func(key string, reader io.Reader, parents []string) {
					data, _ := io.ReadAll(reader)
					mu.Lock()
					pairs = append(pairs, key+"="+strings.Trim(string(data), `"`))
					mu.Unlock()
				},
			),
		)
		require.NoError(t, err)

		mu.Lock()
		require.Len(t, pairs, 6, "iteration %d: expected 6, got %d: %v", iter, len(pairs), pairs)
		mu.Unlock()
	}
}

func TestFieldStreamGoroutineSync_TopLevelObjectMultiField(t *testing.T) {
	input := `{"action":"plan","main_task":"Build app","tasks":[{"name":"step1"},{"name":"step2"}],"goal":"Ship it"}`

	var mu sync.Mutex
	results := make(map[string]string)

	err := ExtractStructuredJSON(input,
		WithRegisterMultiFieldStreamHandler(
			[]string{"action", "main_task", "tasks", "goal"},
			func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				mu.Lock()
				results[key] = string(data)
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, `"plan"`, results["action"])
	assert.Equal(t, `"Build app"`, results["main_task"])
	assert.Equal(t, `"Ship it"`, results["goal"])
	assert.Contains(t, results["tasks"], `"step1"`)
	assert.Contains(t, results["tasks"], `"step2"`)
}

func TestFieldStreamGoroutineSync_LargeArrayOfObjects(t *testing.T) {
	n := 200
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`{"id":"%d","payload":"payload_data_%d_with_some_extra_content"}`, i, i))
	}
	sb.WriteString("]")

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		data := []byte(sb.String())
		chunkSize := 128
		for i := 0; i < len(data); i += chunkSize {
			end := i + chunkSize
			if end > len(data) {
				end = len(data)
			}
			pw.Write(data[i:end])
		}
	}()

	var mu sync.Mutex
	seen := make(map[string]bool)

	err := ExtractStructuredJSONFromStream(pr,
		WithRegisterMultiFieldStreamHandler(
			[]string{"id", "payload"},
			func(key string, reader io.Reader, parents []string) {
				data, _ := io.ReadAll(reader)
				val := strings.Trim(string(data), `"`)
				mu.Lock()
				seen[key+":"+val] = true
				mu.Unlock()
			},
		),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < n; i++ {
		idKey := fmt.Sprintf("id:%d", i)
		payloadKey := fmt.Sprintf("payload:payload_data_%d_with_some_extra_content", i)
		assert.True(t, seen[idKey], "missing: %s", idKey)
		assert.True(t, seen[payloadKey], "missing: %s", payloadKey)
	}
}
