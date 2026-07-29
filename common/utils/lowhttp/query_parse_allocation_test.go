package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func legacyParseQueryParamsForTest(s string, options ...QueryOption) *QueryParams {
	query := &QueryParams{}
	for _, option := range options {
		option(query)
	}

	scanner := bufio.NewReaderSize(bytes.NewBufferString(s), len(s))
	var items []*QueryParamItem
	position := query.Position
	handle := func(pair string) {
		if len(pair) <= 0 {
			return
		}
		pair = strings.Trim(pair, "&")
		key, val, ok := strings.Cut(pair, "=")
		if ok {
			if strings.HasPrefix(key, "{{urlescape(") || strings.HasPrefix(val, "{{urlescape(") {
				key = strings.TrimPrefix(key, "{{urlescape(")
				key = strings.TrimSuffix(key, ")}}")
				val = strings.TrimPrefix(val, "{{urlescape(")
				val = strings.TrimSuffix(val, ")}}")
				pair = fmt.Sprintf("%s=%s", key, val)
			}
			items = append(items, &QueryParamItem{
				Raw:          codec.ForceQueryUnescape(pair),
				Key:          codec.ForceQueryUnescape(key),
				Value:        codec.ForceQueryUnescape(val),
				ValueRaw:     val,
				Position:     position,
				NoAutoEncode: query.NoAutoEncode,
			})
		} else {
			items = append(items, &QueryParamItem{
				Raw:          codec.ForceQueryUnescape(pair),
				Key:          codec.ForceQueryUnescape(key),
				Position:     position,
				NoAutoEncode: query.NoAutoEncode,
			})
		}
	}

	for {
		pair, err := scanner.ReadString('&')
		if err != nil {
			handle(pair)
			break
		}
		handle(pair)
	}
	query.Items = items
	return query
}

func TestParseQueryParamsIndexedMatchesBufferedParser(t *testing.T) {
	inputs := []string{
		"", "&", "&&", "a", "a=", "=b", "a=1&b=2", "a=1&&b=2&",
		"a=b=c", "a+b=c+d", "a%20b=%u4f60%u597d", "bad=%zz",
		"{{urlescape(a)}}={{urlescape(b+c)}}", "中文=值&重复=1&重复=2",
		"a=1\r\nInjected: yes", string([]byte{'a', '=', 0xff, '&', 0, '=', 'b'}),
	}

	random := rand.New(rand.NewSource(41))
	alphabet := []byte("abcXYZ09&=%+{}()[]_-.%u\r\n\x00\xff")
	for sample := 0; sample < 2000; sample++ {
		length := random.Intn(256)
		value := make([]byte, length)
		for index := range value {
			value[index] = alphabet[random.Intn(len(alphabet))]
		}
		inputs = append(inputs, string(value))
	}

	optionSets := [][]QueryOption{
		nil,
		{WithPosition(PosPostQuery)},
		{WithDisableAutoEncode(true), WithPosition(PosGetQuery)},
	}
	for _, input := range inputs {
		for _, options := range optionSets {
			want := legacyParseQueryParamsForTest(input, options...)
			got := ParseQueryParams(input, options...)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("parser mismatch for input %q and %d options:\n got: %#v\nwant: %#v", input, len(options), got, want)
			}
		}
	}
}

func BenchmarkParseQueryParams64KOpaque(b *testing.B) {
	input := strings.Repeat("r", 64*1024)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if got := len(ParseQueryParams(input).Items); got != 1 {
			b.Fatalf("got %d items, want 1", got)
		}
	}
}

func BenchmarkLegacyParseQueryParams64KOpaque(b *testing.B) {
	input := strings.Repeat("r", 64*1024)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if got := len(legacyParseQueryParamsForTest(input).Items); got != 1 {
			b.Fatalf("got %d items, want 1", got)
		}
	}
}
