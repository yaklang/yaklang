package bin_parser

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// Public results can outlive both a parsing call and many subsequent leases of
// private decoder scratch space. Include failed parses and concurrent callers.
func TestStructuredResultLifetime(t *testing.T) {
	t.Run("Node", func(t *testing.T) { testStructuredResultLifetime(t, false) })
	t.Run("Direct", func(t *testing.T) { testStructuredResultLifetime(t, true) })
}

func testStructuredResultLifetime(t *testing.T, direct bool) {
	works := currentCorpusWorks(t, true)
	for worker := 0; worker < 8; worker++ {
		t.Run(string(rune('A'+worker)), func(t *testing.T) {
			t.Parallel()
			var retained []map[string]any
			var snapshots [][]byte
			for round := 0; round < 4; round++ {
				for _, w := range works {
					var value map[string]any
					var err error
					if direct {
						value, err = ParseStructured(w.wire, w.rule, w.entry)
					} else {
						value, err = structuredNodeReference(w.wire, w.rule, w.entry)
					}
					require.NoError(t, err)
					data, err := json.Marshal(value)
					require.NoError(t, err)
					retained, snapshots = append(retained, value), append(snapshots, data)
					if direct {
						_, err = ParseStructured(w.wire[:len(w.wire)-1], w.rule, w.entry)
					} else {
						_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(w.wire[:len(w.wire)-1]), w.rule, w.entry)
					}
					require.Error(t, err)
				}
			}
			for i, value := range retained {
				data, err := json.Marshal(value)
				require.NoError(t, err)
				require.Equal(t, snapshots[i], data, "retained result %d", i)
			}
		})
	}
}
