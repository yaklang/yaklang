package bin_parser

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func TestNativeBridgePublicDifferential(t *testing.T) {
	for _, work := range currentCorpusWorks(t, true) {
		t.Run(work.id, func(t *testing.T) {
			inputs := [][]byte{bytes.Clone(work.wire), append(bytes.Clone(work.wire), 0)}
			for cut := 0; cut < len(work.wire); cut++ {
				inputs = append(inputs, work.wire[:cut])
			}
			for _, wire := range inputs {
				var expected any
				for _, legacy := range []bool{true, false} {
					reader := newProtocolCorpusBoundedReader(wire)
					n, err := parser.ParseBinaryWithConfig(reader, work.rule, map[string]any{"nativeBridgeLegacy": legacy}, work.entry)
					result := []any{fmt.Sprint(err), reader.Len()}
					if err == nil {
						v := map[string]any{"fields": NodeToMap(n), "metadata": n.Cfg.GetItem("additionInfo")}
						result = append(result, v, currentExportTypeShape(reflect.ValueOf(v)), bytes.Clone(NodeToBytes(n)))
					}
					if legacy {
						expected = result
					} else {
						require.Equal(t, expected, result)
					}
				}
			}
		})
	}
}
