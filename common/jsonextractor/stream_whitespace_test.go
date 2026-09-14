package jsonextractor

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
)

var whitespaceDocuments = []struct {
	name string
	raw  string
}{
	{"object", `{"text":"hello world","empty":"","integer":-42,"fraction":1.25,"exponent":-1.25e+3,"yes":true,"no":false,"nothing":null}`},
	{"array", `["hello world","",-42,1.25,-1.25e-3,true,false,null,[],{},[1,2],{"key":"value"}]`},
	{"nested", `{"outer":[{"queries":["ERROR","WARN"],"options":{"enabled":true}},[],{},[null,false]],"after":"intact"}`},
	{"escapes", `{"key with spaces":"line1\n\tline2\r\nline3","punctuation":"{}[],:\"\\","literal":"\\n","unicode":"中文🙂\u4e2d","after":true}`},
	{"json_string_escapes", `{"":"empty key","key\n\t\"\\":"escaped key","solidus":"https:\/\/example.com","controls":"\b\f\n\r\t","surrogate":"\ud83d\ude00","\ud83d\ude00":"surrogate key","number":"-42","boolean":"true","nothing":"null"}`},
	{"empty_object", `{}`},
	{"empty_array", `[]`},
}

var jsonWhitespace = []struct {
	name string
	text string
}{
	{"LF", "\n"},
	{"CRLF", "\r\n"},
	{"CR", "\r"},
	{"tab", "\t"},
	{"space", " "},
	{"mixed", " \t\r\n\n\r\n\t "},
}

// Ask encoding/json where a newline is legal instead of duplicating the
// extractor's state machine. In particular, this excludes positions inside
// strings, escapes, numbers and the true/false/null tokens.
func jsonWhitespaceBoundaries(raw string) []int {
	var positions []int
	for i := 0; i <= len(raw); i++ {
		if json.Valid([]byte(raw[:i] + "\n" + raw[i:])) {
			positions = append(positions, i)
		}
	}
	return positions
}

func insertJSONWhitespace(raw string, positions []int, choose func(int) string) string {
	var out strings.Builder
	previous := 0
	for i, position := range positions {
		out.WriteString(raw[previous:position])
		out.WriteString(choose(i))
		previous = position
	}
	out.WriteString(raw[previous:])
	return out.String()
}

func requireJSONStreamMatchesStandard(t testing.TB, raw string, reader io.Reader, options ...CallbackOption) {
	t.Helper()
	var expected any
	require.NoError(t, json.Unmarshal([]byte(raw), &expected), "test input must be valid JSON: %q", raw)
	var actual any
	// Container callbacks run when each container closes, so the final callback
	// is the root object/array, including for empty containers.
	options = append(options,
		WithObjectCallback(func(value map[string]any) { actual = value }),
		WithArrayCallback(func(value []any) { actual = value }),
	)
	require.NoError(t, ExtractStructuredJSONFromStream(reader, options...), "input: %q", raw)
	encoded, err := json.Marshal(actual)
	require.NoError(t, err)
	var normalized any
	require.NoError(t, json.Unmarshal(encoded, &normalized))
	require.Equal(t, expected, normalized, "input: %q", raw)
}

func TestJSONWhitespaceEveryBoundary(t *testing.T) {
	for _, document := range whitespaceDocuments {
		t.Run(document.name, func(t *testing.T) {
			requireJSONStreamMatchesStandard(t, document.raw, strings.NewReader(document.raw))
			positions := jsonWhitespaceBoundaries(document.raw)
			require.NotEmpty(t, positions)
			for _, whitespace := range jsonWhitespace {
				t.Run(whitespace.name, func(t *testing.T) {
					for _, position := range positions {
						t.Run(fmt.Sprintf("byte_%d", position), func(t *testing.T) {
							raw := document.raw[:position] + whitespace.text + document.raw[position:]
							requireJSONStreamMatchesStandard(t, raw, iotest.OneByteReader(strings.NewReader(raw)))
						})
					}
				})
			}
		})
	}
}

func TestJSONWhitespaceCombinedBoundaries(t *testing.T) {
	for _, document := range whitespaceDocuments {
		for _, whitespace := range jsonWhitespace {
			t.Run(document.name+"/"+whitespace.name, func(t *testing.T) {
				raw := insertJSONWhitespace(document.raw, jsonWhitespaceBoundaries(document.raw), func(int) string {
					return whitespace.text
				})
				for _, mode := range []string{"buffered", "one_byte", "data_with_EOF"} {
					t.Run(mode, func(t *testing.T) {
						var reader io.Reader = strings.NewReader(raw)
						switch mode {
						case "one_byte":
							reader = iotest.OneByteReader(reader)
						case "data_with_EOF":
							reader = iotest.DataErrReader(reader)
						}
						requireJSONStreamMatchesStandard(t, raw, reader)
					})
				}
			})
		}
	}
}

func TestJSONWhitespaceLongRuns(t *testing.T) {
	// Cross common reader buffer sizes, including in the middle of CRLF.
	whitespace := strings.Repeat(" \r\n\t", 2049)
	raw := whitespace + `{"queries"` + whitespace + `:` + whitespace +
		`[` + whitespace + `"ERROR",` + whitespace + `"WARN"` + whitespace +
		`],"after":true}` + whitespace
	requireJSONStreamMatchesStandard(t, raw, iotest.OneByteReader(strings.NewReader(raw)))
}

func TestJSONWhitespaceFieldStreams(t *testing.T) {
	for _, payload := range []string{
		`"line1\n\tline2\r\n中文🙂\\n"`,
		`["ERROR",{"nested":[true,null]},[],{}]`,
		`{"text":"{}[],:\"\\","list":[1,2],"empty":{}}`,
	} {
		t.Run(payload, func(t *testing.T) {
			compact := `{"outer":[{"payload":` + payload + `,"sibling":"intact"}],"after":true}`
			raw := insertJSONWhitespace(compact, jsonWhitespaceBoundaries(compact), func(int) string {
				return " \r\n\t\n"
			})
			type fieldResult struct {
				key     string
				parents []string
				data    []byte
				err     error
			}
			var mu sync.Mutex
			var results []fieldResult
			requireJSONStreamMatchesStandard(t, raw, iotest.OneByteReader(strings.NewReader(raw)),
				WithRegisterFieldStreamHandler("payload", func(key string, reader io.Reader, parents []string) {
					data, err := io.ReadAll(reader)
					mu.Lock()
					defer mu.Unlock()
					results = append(results, fieldResult{key, append([]string(nil), parents...), data, err})
				}),
			)
			// Extraction waits for field handlers; no sleeps or polling are needed.
			mu.Lock()
			defer mu.Unlock()
			require.Len(t, results, 1)
			require.NoError(t, results[0].err)
			require.Equal(t, "payload", results[0].key)
			// Parent paths contain object keys; array indexes are not part of this API.
			require.Equal(t, []string{"outer"}, results[0].parents)
			require.JSONEq(t, payload, string(results[0].data))
		})
	}
}

func TestJSONWhitespaceNumericValues(t *testing.T) {
	for _, number := range []string{"0", "-0", "42", "-42", "0.0", "-1.25", "1e3", "1E-3", "-1.25e+3", "9223372036854775808"} {
		t.Run(number, func(t *testing.T) {
			raw := "{\"number\":\r\n\t" + number + "\n,\"quoted\":\"" + number + "\",\"after\":true}"
			requireJSONStreamMatchesStandard(t, raw, iotest.OneByteReader(strings.NewReader(raw)))
		})
	}
	for _, malformed := range []string{"1.25trailing", "1.2e+", "1e9999"} {
		t.Run(malformed, func(t *testing.T) {
			_, value, _, _ := rawValueFormatter("\r\n\t" + malformed + "\n")
			require.Equal(t, malformed, value, "preserve unconvertible text instead of returning zero or infinity")
		})
	}
}

func TestJSONWhitespacePreservesRecovery(t *testing.T) {
	// The extractor intentionally tolerates missing commas and literal control
	// bytes in strings. Legal whitespace support must not remove that behavior.
	for _, value := range []string{`42`, `true`, `null`, `"text"`, `[1,2]`, `{"nested":true}`} {
		for _, newline := range []string{"\n", "\r\n\t\n"} {
			t.Run(value+fmt.Sprintf("/%q", newline), func(t *testing.T) {
				raw := `{"value":` + newline + value + newline + `"after":"intact"}`
				var actual map[string]any
				require.NoError(t, ExtractStructuredJSONFromStream(iotest.OneByteReader(strings.NewReader(raw)),
					WithRootMapCallback(func(value map[string]any) { actual = value }),
				))
				require.JSONEq(t, `{"value":`+value+`,"after":"intact"}`, mustJSON(t, actual))
			})
		}
	}
	for _, quoted := range []string{"\"line1\n\tline2\r\nline3\"", `"\x41"`} {
		_, value, _, _ := rawValueFormatter(quoted)
		if strings.Contains(quoted, "\n") {
			require.Equal(t, "line1\n\tline2\r\nline3", value)
		} else {
			require.Equal(t, "A", value)
		}
	}
}

func TestJSONWhitespaceStreamsBeforeEOF(t *testing.T) {
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	firstItem := "[\r\n\t\"first\""
	progress := make(chan error, 1)
	parsed := make(chan error, 1)
	go func() {
		parsed <- ExtractStructuredJSONFromStream(input,
			WithRegisterFieldStreamHandler("payload", func(_ string, reader io.Reader, _ []string) {
				prefix := make([]byte, len(firstItem))
				_, err := io.ReadFull(reader, prefix)
				if err == nil && string(prefix) != firstItem {
					err = fmt.Errorf("unexpected field prefix: %q", prefix)
				}
				progress <- err
				_, _ = io.Copy(io.Discard, reader)
			}),
		)
	}()
	written := make(chan error, 1)
	go func() {
		// Supply a newline for the parser's lookahead after the closing quote.
		_, err := io.WriteString(writer, "{\n\"payload\"\r\n:\n"+firstItem+"\n")
		written <- err
	}()
	select {
	case err := <-progress:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("field handler did not receive the first array item while input remained open")
	}
	require.NoError(t, <-written)
	go func() {
		_, err := io.WriteString(writer, ",\r\n\"second\"\n]\n,\"after\":true\n}")
		closeErr := writer.Close()
		if err == nil {
			err = closeErr
		}
		written <- err
	}()
	select {
	case err := <-parsed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("extractor did not finish after EOF")
	}
	require.NoError(t, <-written)
}

func FuzzJSONWhitespace(f *testing.F) {
	for i := range whitespaceDocuments {
		f.Add(uint8(i), []byte{0, 1, 2, 3, 4, 5})
		f.Add(uint8(i), []byte{5, 0, 5, 1, 5})
	}
	f.Fuzz(func(t *testing.T, documentIndex uint8, schedule []byte) {
		document := whitespaceDocuments[int(documentIndex)%len(whitespaceDocuments)]
		raw := insertJSONWhitespace(document.raw, jsonWhitespaceBoundaries(document.raw), func(i int) string {
			if len(schedule) == 0 {
				return ""
			}
			choice := int(schedule[i%len(schedule)]) % (len(jsonWhitespace) + 1)
			if choice == len(jsonWhitespace) {
				return ""
			}
			return jsonWhitespace[choice].text
		})
		requireJSONStreamMatchesStandard(t, raw, iotest.OneByteReader(strings.NewReader(raw)))
	})
}
