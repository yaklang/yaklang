package locktoml

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

func TestUpstreamSyntaxCorpus(t *testing.T) {
	err := fixtures.WalkDir("testdata/upstream", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		path = filepath.ToSlash(path)
		if d.IsDir() || !strings.HasSuffix(path, ".toml") || strings.Contains(path, "/valid-next/") || strings.Contains(path, "/invalid-next/") {
			return nil
		}
		t.Run(strings.TrimPrefix(path, "testdata/upstream/"), func(t *testing.T) {
			b, err := fixtures.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Parse(context.Background(), b)
			if strings.Contains(path, "/invalid/") {
				if err == nil {
					t.Fatalf("invalid TOML accepted: %s", b)
				}
				return
			}
			expected, e := fixtures.ReadFile(strings.TrimSuffix(path, ".toml") + ".json")
			if e != nil {
				t.Fatal(e)
			}
			var want any
			if e = json.Unmarshal(expected, &want); e != nil {
				t.Fatal(e)
			}
			// These fixtures are explicitly marked TOML 1.1 by upstream's
			// TestTomlNextFails. The frozen lock grammar is TOML 1.0.
			nextOnly := map[string]bool{"valid/string/escape-esc.toml": true, "valid/string/hex-escape.toml": true, "valid/inline-table/newline.toml": true, "valid/key/unicode.toml": true, "valid/datetime/no-seconds.toml": true}
			if nextOnly[strings.TrimPrefix(path, "testdata/upstream/")] {
				if err == nil {
					t.Fatal("accepted unfrozen TOML 1.1 syntax")
				}
				return
			}
			if hasDate(want) {
				if err == nil || !strings.Contains(err.Error(), "unsupported_syntax") {
					t.Fatalf("dates must be explicitly unsupported, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if e = compareTyped(got, want); e != nil {
				t.Fatal(e)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func hasDate(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		if typ, ok := x["type"].(string); ok && (strings.Contains(typ, "date") || strings.Contains(typ, "time")) {
			return true
		}
		for _, v := range x {
			if hasDate(v) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if hasDate(v) {
				return true
			}
		}
	}
	return false
}
func compareTyped(got, want any) error {
	switch w := want.(type) {
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return fmt.Errorf("array got %v want %v", got, want)
		}
		for i := range w {
			if e := compareTyped(g[i], w[i]); e != nil {
				return e
			}
		}
		return nil
	case map[string]any:
		if typ, ok := w["type"].(string); ok {
			value := w["value"].(string)
			var expected any
			switch typ {
			case "string":
				expected = value
			case "bool":
				expected = value == "true"
			case "integer":
				expected, _ = strconv.ParseInt(value, 10, 64)
			case "float":
				f, _ := strconv.ParseFloat(value, 64)
				if math.IsNaN(f) {
					if g, ok := got.(float64); ok && math.IsNaN(g) {
						return nil
					}
				}
				expected = f
			default:
				return fmt.Errorf("unmapped oracle type %s", typ)
			}
			if !reflect.DeepEqual(got, expected) {
				return fmt.Errorf("got %#v want %#v", got, expected)
			}
			return nil
		}
		g, ok := got.(map[string]any)
		if !ok || len(g) != len(w) {
			return fmt.Errorf("mapping got %v want %v", got, want)
		}
		for k, v := range w {
			if e := compareTyped(g[k], v); e != nil {
				return fmt.Errorf("%s: %w", k, e)
			}
		}
		return nil
	}
	return fmt.Errorf("unmapped oracle value")
}
func TestReadRecordsBudgetBeforeMaterialization(t *testing.T) {
	raw := []byte("[[package]]\nname='a'\nversion='1.2.3'\n")
	l, err := (budget.Limits{MaxResultBytes: 512}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	m, spans, err := ReadRecords(budget.Bind(context.Background(), l), bytes.NewReader(raw), "package")
	if !errors.Is(err, scanerr.ErrResourceLimit) || m != nil || spans != nil {
		t.Fatalf("returned partial records: %v %v %v", m, spans, err)
	}
	m, spans, err = ReadRecords(context.Background(), bytes.NewReader(raw), "package")
	if err != nil || len(spans) != 1 || spans[0].StartLine != 1 || spans[0].EndLine != 3 {
		t.Fatalf("positive: %v %v %v", m, spans, err)
	}
	packages := m["package"].([]any)
	if packages[0].(map[string]any)["name"] != "a" {
		t.Fatalf("lost table: %v", m)
	}
}

func TestBudgetsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Parse(ctx, []byte("a=1")); e == nil {
		t.Fatal("cancel ignored")
	}
	if _, e := Parse(context.Background(), []byte("a="+strings.Repeat("[", 200)+"0"+strings.Repeat("]", 200))); e == nil {
		t.Fatal("depth ignored")
	}
}
func FuzzLockTOML(f *testing.F) {
	f.Add([]byte("[[package]]\nname='a'\nversion='1.2.3'\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		Parse(context.Background(), b)
	})
}
