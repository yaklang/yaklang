//go:build linux

package inputresolver

import (
	"context"
	"strings"
	"testing"
)

func TestManagedInputUnicodeReadAndSearchOffsets(t *testing.T) {
	contents := []string{"中文🙂tail", "Kprefix MATCH here\n", strings.Repeat("x", (64<<10)-2) + "Kelvin tail\n"}
	m, id, refs := manifestFixture(contents...)
	w, err := resolverFixture(t, nil).Prepare(context.Background(), m, id, refs, downloadFixture(t, contents...), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Cleanup()
	t.Run("read from continuation byte", func(t *testing.T) {
		got, err := w.Read(context.Background(), m.Resources[0].RelativePath, 1, 4)
		if err != nil || got["content"] != "文" || got["offset"] != int64(3) || got["next_offset"] != int64(6) {
			t.Fatalf("read=%v err=%v", got, err)
		}
	})
	t.Run("page smaller than a rune", func(t *testing.T) {
		got, err := w.Read(context.Background(), m.Resources[0].RelativePath, 0, 1)
		if err == nil {
			t.Fatalf("empty non-progressing page accepted: %v", got)
		}
	})
	for _, tc := range []struct {
		name, path, query string
		offset            int64
	}{
		{"case fold preserves original offset", m.Resources[1].RelativePath, "match", int64(len("Kprefix "))},
		{"Unicode fold across buffer boundary", m.Resources[2].RelativePath, "kelvin", (64 << 10) - 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := w.Search(context.Background(), tc.path, tc.query, false, 10)
			if err != nil {
				t.Fatal(err)
			}
			matches := got["matches"].([]map[string]any)
			if len(matches) != 1 || matches[0]["offset"] != tc.offset {
				t.Fatalf("search=%v", got)
			}
		})
	}
}
