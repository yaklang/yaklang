package bin_parser

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// JSON preserves values but cannot distinguish uint8(1) from uint64(1).
// Add concrete type and nilness evidence without recording pointer addresses.
func currentExportTypeShape(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface {
		return currentExportTypeShape(v.Elem())
	}
	result := map[string]any{"type": v.Type().String()}
	switch v.Kind() {
	case reflect.Map:
		result["nil"] = v.IsNil()
		children := map[string]any{}
		for _, key := range v.MapKeys() {
			children[fmt.Sprint(key.Interface())] = currentExportTypeShape(v.MapIndex(key))
		}
		result["children"] = children
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice {
			result["nil"] = v.IsNil()
		}
		if v.Type().Elem().Kind() != reflect.Uint8 {
			children := make([]any, v.Len())
			for i := range children {
				children[i] = currentExportTypeShape(v.Index(i))
			}
			result["children"] = children
		}
	}
	return result
}

// Opt-in cross-binary evidence for storage/engine experiments. Each fixed
// partition hashes public fields, metadata, concrete value types, errors and
// final reader position for every retained record. Independent field tests
// still determine whether those outputs are correct, not merely unchanged.
func TestProtocolCorpusExportDigest(t *testing.T) {
	if os.Getenv("BINPARSER_EVALUATE") != "1" {
		t.Skip("manual cross-binary output equivalence")
	}
	for _, application := range []bool{false, true} {
		label := "envelopes"
		if application {
			label = "application"
		}
		works := currentCorpusWorks(t, application)
		t.Run(label, func(t *testing.T) {
			for worker := 0; worker < 4; worker++ {
				worker := worker
				t.Run(fmt.Sprint(worker), func(t *testing.T) {
					t.Parallel()
					digest, count := sha256.New(), 0
					for i := worker; i < len(works); i += 4 {
						w := works[i]
						reader := newProtocolCorpusBoundedReader(w.wire)
						n, err := parser.ParseBinary(reader, w.rule, w.entry)
						record := map[string]any{"id": w.id, "rule": w.rule, "entry": w.entry, "remaining": reader.Len()}
						if w.rejection != "" {
							if err == nil || !strings.Contains(err.Error(), w.rejection) || n != nil {
								t.Fatalf("unexpected rejection %s: %v", w.id, err)
							}
							record["error"] = err.Error()
						} else {
							if err != nil || reader.Len() != 0 {
								t.Fatalf("unexpected parse %s: %v / %d", w.id, err, reader.Len())
							}
							value := map[string]any{"fields": NodeToMap(n), "metadata": n.Cfg.GetItem("additionInfo")}
							record["value"] = value
							record["types"] = currentExportTypeShape(reflect.ValueOf(value))
						}
						data, err := json.Marshal(record)
						if err != nil {
							t.Fatalf("digest %s: %v", w.id, err)
						}
						digest.Write(data)
						digest.Write([]byte{'\n'})
						count++
					}
					t.Logf("CURRENT_EXPORT_DIGEST workload=%s worker=%d records=%d sha256=%x", label, worker, count, digest.Sum(nil))
				})
			}
		})
	}
}
