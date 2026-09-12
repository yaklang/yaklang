package bin_parser

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func structuredNodeReference(wire []byte, rule string, keys ...string) (map[string]any, error) {
	node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), rule, keys...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"fields": NodeToMap(node), "metadata": node.Cfg.GetItem("additionInfo")}, nil
}

func TestParseStructuredCorpusEquivalence(t *testing.T) {
	for _, work := range currentCorpusWorks(t, true) {
		t.Run(work.id+"/"+work.entry, func(t *testing.T) {
			wire := bytes.Clone(work.wire)
			want, err := structuredNodeReference(wire, work.rule, work.entry)
			require.NoError(t, err)
			got, err := ParseStructured(wire, work.rule, work.entry)
			require.NoError(t, err)
			require.Equal(t, want, got) // Includes concrete integer and collection types.
			clear(wire)
			require.Equal(t, want, got, "returned fields and metadata must own their input bytes")
			for length := 0; length < len(work.wire); length++ {
				compareStructuredOutcome(t, work.wire[:length], work.rule, work.entry)
			}
			compareStructuredOutcome(t, append(bytes.Clone(work.wire), 0), work.rule, work.entry)
			for index := range work.wire {
				mutated := bytes.Clone(work.wire)
				mutated[index] ^= 0xff
				compareStructuredOutcome(t, mutated, work.rule, work.entry)
			}
		})
	}
}

func compareStructuredOutcome(t *testing.T, wire []byte, rule string, keys ...string) {
	t.Helper()
	want, wantErr := structuredNodeReference(wire, rule, keys...)
	got, err := ParseStructured(wire, rule, keys...)
	if wantErr != nil {
		require.EqualError(t, err, wantErr.Error())
		require.Nil(t, got)
	} else {
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestParseStructuredFallback(t *testing.T) {
	const rule = "application-layer.memcached_fields"
	for _, wire := range [][]byte{[]byte("stats\r\n"), []byte("unrecognized\r\n")} {
		compareStructuredOutcome(t, wire, rule, "MemcachedStatsRequestFieldsCarrier")
	}
	value, err := ParseStructured(make([]byte, 48), "application-layer.ntp", "NTP")
	require.NoError(t, err)
	require.NotNil(t, value)
	compareStructuredOutcome(t, make([]byte, 48), "application-layer.ntp", "NTP")
	compareStructuredOutcome(t, []byte("stats\r\n"), rule, "missing")
	compareStructuredOutcome(t, []byte("stats\r\n"), "missing-rule", "Missing")
	// Multi-component paths retain the original entry resolution and diagnostics.
	compareStructuredOutcome(t, []byte("stats\r\n"), rule, "Package", "MemcachedStatsRequestFields")
	compareStructuredOutcome(t, []byte("stats\r\n"), rule)
}

func TestParseStructuredCompleteBoundary(t *testing.T) {
	reader := newProtocolCorpusBoundedReader(make([]byte, 51))
	_, err := parser.ParseBinary(reader, "application-layer.ntp", "NTP")
	require.NoError(t, err)
	require.Equal(t, 3, reader.Len())
	value, err := ParseStructured(make([]byte, 51), "application-layer.ntp", "NTP")
	require.EqualError(t, err, "structured parse: 3 unconsumed message bytes")
	require.Nil(t, value)
}

func TestParseStructuredEmptyCollections(t *testing.T) {
	compareStructuredOutcome(t, []byte("END\r\n"), "application-layer.memcached_fields", "MemcachedStatsResponseFields")
	// SUPPORTED with zero options, and with one option whose value list is empty.
	for _, wire := range [][]byte{
		{0x84, 0, 0, 0, 6, 0, 0, 0, 2, 0, 0},
		{0x84, 0, 0, 0, 6, 0, 0, 0, 7, 0, 1, 0, 1, 'X', 0, 0},
	} {
		compareStructuredOutcome(t, wire, "application-layer.cassandra_fields", "CQLSupported4Fields")
	}
}

func TestParseStructuredMutableValuesAreIndependent(t *testing.T) {
	for _, work := range currentCorpusWorks(t, true) {
		t.Run(work.id, func(t *testing.T) {
			value, err := ParseStructured(work.wire, work.rule, work.entry)
			require.NoError(t, err)
			var values [][]byte
			var collect func(any)
			collect = func(value any) {
				switch value := value.(type) {
				case []byte:
					values = append(values, value)
				case map[string]any:
					for _, child := range value {
						collect(child)
					}
				case []any:
					for _, child := range value {
						collect(child)
					}
				case []map[string]any:
					for _, child := range value {
						collect(child)
					}
				}
			}
			collect(value)
			snapshots := make([][]byte, len(values))
			for i, value := range values {
				snapshots[i] = bytes.Clone(value)
			}
			for i, value := range values {
				if len(value) == 0 {
					continue
				}
				value[0] ^= 0xff
				for j := range values {
					if i != j {
						require.Equal(t, snapshots[j], values[j], "mutating byte value %d changed %d", i, j)
					}
				}
				value[0] ^= 0xff
				extended := append(value, 0xff) // Must not write into another exported value.
				require.Equal(t, byte(0xff), extended[len(value)])
				for j := range values {
					require.Equal(t, snapshots[j], values[j])
				}
			}
			want, err := structuredNodeReference(work.wire, work.rule, work.entry)
			require.NoError(t, err)
			require.Equal(t, want, value, "byte mutation must not change immutable text")
		})
	}
}

func TestParseStructuredLargeRepeatedFields(t *testing.T) {
	for _, line := range []string{"STAT same 123\r\n", "STAT same " + strings.Repeat("x", 1100) + "\r\n"} {
		// Exercise repeated keys, growth past the initial reservation and the
		// large-message text ownership path without changing the benchmark corpus.
		wire := []byte(strings.Repeat(line, 65) + "END\r\n")
		compareStructuredOutcome(t, wire, "application-layer.memcached_fields", "MemcachedStatsResponseFields")
		compareStructuredOutcome(t, wire[:len(wire)-1], "application-layer.memcached_fields", "MemcachedStatsResponseFields")
	}
}
