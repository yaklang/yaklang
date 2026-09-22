package cargo

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/locktoml"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The removed JSON bridge is an independent test-only oracle for every frozen
// lock file, including source/checksum/marker/file metadata, not just identities.
func TestFixedLockMappingAgainstJSONOracle(t *testing.T) {
	count := 0
	err := fixtures.WalkDir("testdata", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".lock") {
			return nil
		}
		t.Run(path, func(t *testing.T) {
			raw, err := fixtures.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			m, err := locktoml.Parse(context.Background(), raw)
			if filepath.Base(path) == "cargo_invalid.lock" {
				if scanerr.CodeOf(err) != scanerr.MalformedInput {
					t.Fatalf("missing-value fixture: %v", err)
				}
				count++
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			var want Lockfile
			oracleErr := json.Unmarshal(encoded, &want)
			got, err := decodeLock(context.Background(), m)
			if (err != nil) != (oracleErr != nil) {
				t.Fatalf("error mismatch: new=%v old=%v", err, oracleErr)
			}
			if err == nil && !reflect.DeepEqual(got, want) {
				t.Fatalf("full DTO differs: got=%#v want=%#v", got, want)
			}
			count++
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no frozen locks tested")
	}
	for _, bad := range []map[string]any{{"package": "bad"}, {"package": []any{"bad"}}, {"package": []any{map[string]any{"name": true}}}} {
		if _, err := decodeLock(context.Background(), bad); err == nil {
			t.Fatalf("accepted wrong field type: %v", bad)
		}
	}
	records := make([]any, 10000)
	for i := range records {
		records[i] = map[string]any{"name": "p", "version": "1"}
	}
	limits, _ := (budget.Limits{MaxResultBytes: 128}).Normalize()
	got, err := decodeLock(budget.Bind(context.Background(), limits), map[string]any{"package": records})
	if !errors.Is(err, scanerr.ErrResourceLimit) || got.Packages != nil {
		t.Fatalf("allocated packages before budget: count=%d err=%v", len(got.Packages), err)
	}
}
