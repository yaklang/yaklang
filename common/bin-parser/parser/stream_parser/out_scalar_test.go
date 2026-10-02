package stream_parser

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func outScalarChild(value any) *base.NodeValue {
	return &base.NodeValue{Value: value}
}

func outScalarData(list bool, children ...*base.NodeValue) *base.NodeValue {
	return &base.NodeValue{ListValue: list, Value: children}
}

func outScalarLongValue(value uint64) any {
	if value > uint64(^uint(0)>>1) {
		return int64(value)
	}
	return int(value)
}

func TestEvalScalarOutSourcesMatchIECDefinitions(t *testing.T) {
	fixtures := outProgramSVFixtures(t)
	require.Len(t, fixtures, 3)
	require.Equal(t, outScalarNumberSource, fixtures[0].source)
	require.Equal(t, outScalarBytesSource, fixtures[1].source)
	require.Equal(t, outScalarLengthSource, fixtures[2].source)
}

func TestEvalScalarOutMatchesYakValuesAndTypes(t *testing.T) {
	nilBytes := []byte(nil)
	emptyBytes := make([]byte, 0)
	rawBytes := []byte{0, 1, 0xfe, 0xff}
	cases := []struct {
		name   string
		source string
		data   *base.NodeValue
		want   any
	}{
		{
			name:   "number-zero",
			source: outScalarNumberSource,
			data: outScalarData(false,
				outScalarChild(uint8(0x82)), outScalarChild(uint8(1)), outScalarChild(uint64(0))),
			want: uint64(0),
		},
		{
			name:   "number-five-octets-and-ignored-extra",
			source: outScalarNumberSource,
			data: outScalarData(false,
				outScalarChild(uint8(0x82)), outScalarChild(uint8(5)),
				outScalarChild(uint64(0xffffffffff)), nil),
			want: uint64(0xffffffffff),
		},
		{
			name:   "bytes-absent",
			source: outScalarBytesSource,
			data:   outScalarData(false, nil, nil),
			want:   "",
		},
		{
			name:   "bytes-nil-slice",
			source: outScalarBytesSource,
			data: outScalarData(false,
				outScalarChild(uint8(0x87)), outScalarChild(uint8(0)), outScalarChild(nilBytes)),
			want: nilBytes,
		},
		{
			name:   "bytes-empty-non-nil-slice",
			source: outScalarBytesSource,
			data: outScalarData(false,
				outScalarChild(uint8(0x87)), outScalarChild(uint8(0)), outScalarChild(emptyBytes)),
			want: emptyBytes,
		},
		{
			name:   "bytes-value-and-ignored-extra",
			source: outScalarBytesSource,
			data: outScalarData(false,
				outScalarChild(uint8(0x87)), outScalarChild(uint8(4)), outScalarChild(rawBytes), nil),
			want: rawBytes,
		},
		{
			name:   "length-short-zero",
			source: outScalarLengthSource,
			data:   outScalarData(true, outScalarChild(uint8(0))),
			want:   uint8(0),
		},
		{
			name:   "length-short-max-ignores-nil-tail",
			source: outScalarLengthSource,
			data:   outScalarData(true, outScalarChild(uint8(127)), nil),
			want:   uint8(127),
		},
		{
			name:   "length-long-one-octet-zero",
			source: outScalarLengthSource,
			data:   outScalarData(true, outScalarChild(uint8(0x81)), outScalarChild(uint8(0))),
			want:   outScalarLongValue(0),
		},
		{
			name:   "length-long-two-octets",
			source: outScalarLengthSource,
			data: outScalarData(true,
				outScalarChild(uint8(0x82)), outScalarChild(uint8(1)), outScalarChild(uint8(2))),
			want: outScalarLongValue(258),
		},
		{
			name:   "length-long-leading-zero-is-not-normalized-away",
			source: outScalarLengthSource,
			data: outScalarData(true,
				outScalarChild(uint8(0x82)), outScalarChild(uint8(0)), outScalarChild(uint8(127))),
			want: outScalarLongValue(127),
		},
		{
			name:   "length-long-four-octets-host-int-boundary",
			source: outScalarLengthSource,
			data: outScalarData(true,
				outScalarChild(uint8(0x84)), outScalarChild(uint8(0xff)), outScalarChild(uint8(0xff)),
				outScalarChild(uint8(0xff)), outScalarChild(uint8(0xff))),
			want: outScalarLongValue(0xffffffff),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := outProgramUncached(outProgramSVEngine(tc.data), tc.source)
			require.NoError(t, err)
			require.Equal(t, tc.want, want, "Yak oracle returned a different concrete type or value")

			got, handled := evalScalarOut(tc.source, tc.data)
			require.True(t, handled)
			require.Equal(t, want, got, "native scalar result must preserve the Yak concrete type")
		})
	}

	oracle, err := outProgramUncached(outProgramSVEngine(cases[5].data), outScalarBytesSource)
	require.NoError(t, err)
	oracleBytes := oracle.([]byte)
	require.True(t, &rawBytes[0] == &oracleBytes[0], "the Yak expression returns the original byte-slice backing array")
	got, handled := evalScalarOut(outScalarBytesSource, cases[5].data)
	require.True(t, handled)
	gotBytes := got.([]byte)
	require.True(t, &rawBytes[0] == &gotBytes[0], "the scalar path must preserve the same byte-slice alias")
	require.Nil(t, cases[3].want.([]byte))
	require.NotNil(t, cases[4].want.([]byte))
}

func TestEvalScalarOutConservativeFallbackGuards(t *testing.T) {
	wrongLongWidth := outScalarData(true,
		outScalarChild(uint8(0x82)), outScalarChild(uint8(1)))
	fiveOctetLong := outScalarData(true,
		outScalarChild(uint8(0x85)), outScalarChild(uint8(1)), outScalarChild(uint8(2)),
		outScalarChild(uint8(3)), outScalarChild(uint8(4)), outScalarChild(uint8(5)))
	cases := []struct {
		name   string
		source string
		data   *base.NodeValue
	}{
		{"source-whitespace", outScalarNumberSource + " ", outScalarData(false, nil, nil, outScalarChild(uint64(1)))},
		{"nil-data", outScalarNumberSource, nil},
		{"non-node-value-slice", outScalarNumberSource, &base.NodeValue{Value: []any{1, 2, 3}}},
		{"number-too-short", outScalarNumberSource, outScalarData(false, outScalarChild(uint8(2)), outScalarChild(uint8(1)))},
		{"number-nil-third", outScalarNumberSource, outScalarData(false, outScalarChild(uint8(2)), outScalarChild(uint8(1)), nil)},
		{"number-wrong-type", outScalarNumberSource, outScalarData(false, outScalarChild(uint8(2)), outScalarChild(uint8(1)), outScalarChild(int64(1)))},
		{"bytes-nil-third", outScalarBytesSource, outScalarData(false, outScalarChild(uint8(2)), outScalarChild(uint8(1)), nil)},
		{"bytes-wrong-type", outScalarBytesSource, outScalarData(false, outScalarChild(uint8(2)), outScalarChild(uint8(1)), outScalarChild("value"))},
		{"length-empty", outScalarLengthSource, outScalarData(true)},
		{"length-nil-first", outScalarLengthSource, outScalarData(true, nil)},
		{"length-wrong-first-type", outScalarLengthSource, outScalarData(true, outScalarChild(int(127)))},
		{"length-indefinite", outScalarLengthSource, outScalarData(true, outScalarChild(uint8(0x80)))},
		{"length-count-mismatch", outScalarLengthSource, wrongLongWidth},
		{"length-width-five", outScalarLengthSource, fiveOctetLong},
		{"length-nil-tail", outScalarLengthSource, outScalarData(true, outScalarChild(uint8(0x81)), nil)},
		{"length-wrong-tail-type", outScalarLengthSource, outScalarData(true, outScalarChild(uint8(0x81)), outScalarChild(uint16(1)))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, handled := evalScalarOut(tc.source, tc.data)
			require.False(t, handled)
		})
	}
}

func TestEvalScalarOutConcurrentIsolation(t *testing.T) {
	cases := []struct {
		source string
		data   *base.NodeValue
		want   any
	}{
		{outScalarNumberSource, outScalarData(false, nil, nil, outScalarChild(uint64(0x0102030405))), uint64(0x0102030405)},
		{outScalarBytesSource, outScalarData(false, nil, nil, outScalarChild([]byte{1, 2, 3})), []byte{1, 2, 3}},
		{outScalarLengthSource, outScalarData(true, outScalarChild(uint8(127)), nil), uint8(127)},
		{outScalarLengthSource, outScalarData(true, outScalarChild(uint8(0x84)), outScalarChild(uint8(1)), outScalarChild(uint8(2)), outScalarChild(uint8(3)), outScalarChild(uint8(4))), outScalarLongValue(0x01020304)},
	}

	const workers = 16
	const iterations = 100
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				tc := cases[(worker+iteration)%len(cases)]
				got, handled := evalScalarOut(tc.source, tc.data)
				if !handled || !reflect.DeepEqual(tc.want, got) {
					errors <- fmt.Errorf("worker %d iteration %d: got %#v (%T), handled %v; want %#v (%T)",
						worker, iteration, got, got, handled, tc.want, tc.want)
					return
				}
			}
		}(worker)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

var (
	outScalarBenchmarkValue   any
	outScalarBenchmarkHandled bool
)

func BenchmarkEvalScalarOut(b *testing.B) {
	cases := []struct {
		name   string
		source string
		data   *base.NodeValue
	}{
		{"number", outScalarNumberSource, outScalarData(false, nil, nil, outScalarChild(uint64(0x0102030405)))},
		{"bytes", outScalarBytesSource, outScalarData(false, nil, nil, outScalarChild([]byte{1, 2, 3, 4}))},
		{"length-short", outScalarLengthSource, outScalarData(true, outScalarChild(uint8(127)))},
		{"length-long", outScalarLengthSource, outScalarData(true, outScalarChild(uint8(0x84)), outScalarChild(uint8(1)), outScalarChild(uint8(2)), outScalarChild(uint8(3)), outScalarChild(uint8(4)))},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			value, handled := evalScalarOut(tc.source, tc.data)
			if !handled {
				b.Fatal("fixture did not use scalar path")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				value, handled = evalScalarOut(tc.source, tc.data)
			}
			outScalarBenchmarkValue = value
			outScalarBenchmarkHandled = handled
		})
	}
}
