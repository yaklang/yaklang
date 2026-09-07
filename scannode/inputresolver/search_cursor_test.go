//go:build linux

package inputresolver

import (
	"context"
	"testing"
)

func TestManagedInputSearchCursorUTF8AndFileBoundary(t *testing.T) {
	contents := []string{"中文needle\n", "needle\n"}
	m, id, refs := manifestFixture(contents...)
	var access Event
	w, err := resolverFixture(t, nil).Prepare(context.Background(), m, id, refs, downloadFixture(t, contents...), func(kind string, e Event) {
		if kind == "input.file.access" && e.Operation == "search" {
			access = e
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Cleanup()
	path := m.Resources[0].RelativePath
	got, err := w.SearchFrom(context.Background(), path, "needle", true, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	matches := got["matches"].([]map[string]any)
	if len(matches) != 1 || matches[0]["offset"] != int64(len("中文")) || matches[0]["line"] != nil {
		t.Fatalf("resumed UTF-8 offset or line attribution: %#v", got)
	}
	if access.Offset != int64(len("中")) || access.EndOffset != int64(len(contents[0])) || access.BytesRead != int64(len(contents[0])-len("中")) || access.StartLine != 0 {
		t.Fatalf("resumed access range: %#v", access)
	}
	got, err = w.Search(context.Background(), "inputs", "needle", true, 1)
	if err != nil || got["complete"] != false || got["next_path"] != m.Resources[1].RelativePath || got["next_offset"] != int64(0) {
		t.Fatalf("next file continuation: %#v %v", got, err)
	}
	got, err = w.SearchFrom(context.Background(), path, "needle", true, 10, int64(len(contents[0])))
	if err != nil || got["complete"] != true || got["count"] != 0 || got["scanned_bytes"] != int64(0) {
		t.Fatalf("EOF cursor: %#v %v", got, err)
	}
}
