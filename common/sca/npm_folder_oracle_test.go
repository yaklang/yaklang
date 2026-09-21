package sca

import (
	"context"
	"encoding/json"
	"io/fs"
	"path"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/model"
)

// Read the frozen producer JSON directly, independently of all SCA parsers.
// Count equality alone missed incorrect versions and merged installation paths.
func TestNPMFolderProducerFields(t *testing.T) {
	const base = "testdata/node_npm/positive_folder/"
	input := fstest.MapFS{}
	err := fs.WalkDir(testFS, strings.TrimSuffix(base, "/"), func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := testFS.ReadFile(name)
		if err != nil {
			return err
		}
		input[strings.ReplaceAll(strings.TrimPrefix(name, base), "test_node_modules", "node_modules")] = &fstest.MapFile{Data: raw}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	type record struct{ Name, Version, Source, Kind string }
	want := map[[2]string]record{}
	type declaration struct{ File, Root, Name, Constraint, Scope string }
	var declarations []declaration
	exact := regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	for file, data := range input {
		if path.Base(file) != "package.json" {
			continue
		}
		var raw struct {
			Name, Version                                       string
			Dependencies, OptionalDependencies, DevDependencies map[string]string
		}
		if err := json.Unmarshal(data.Data, &raw); err != nil {
			t.Fatal(err)
		}
		root := file + "#" + raw.Name + "@" + raw.Version
		want[[2]string{file, root}] = record{raw.Name, raw.Version, "", "declared"}
		for _, scope := range []struct {
			name string
			deps map[string]string
			emit bool
		}{{"runtime", raw.Dependencies, true}, {"optional", raw.OptionalDependencies, true}, {"dev", raw.DevDependencies, false}} {
			for name, version := range scope.deps {
				declarations = append(declarations, declaration{file, root, name, version, scope.name})
				if !scope.emit {
					continue
				}
				if !exact.MatchString(version) {
					version = ""
				}
				want[[2]string{file, file + "#declaration:" + name}] = record{name, version, "", "declared"}
			}
		}
	}
	type locked struct {
		Version, Resolved string
		Dependencies      map[string]locked
	}
	var lock struct{ Dependencies map[string]locked }
	if err := json.Unmarshal(input["package-lock.json"].Data, &lock); err != nil {
		t.Fatal(err)
	}
	var visit func(string, map[string]locked)
	visit = func(parent string, deps map[string]locked) {
		for name, p := range deps {
			native := path.Join(parent, "node_modules", name)
			want[[2]string{"package-lock.json", "package-lock.json#" + native}] = record{name, p.Version, p.Resolved, "locked"}
			visit(native, p.Dependencies)
		}
	}
	visit("", lock.Dependencies)
	r, err := ScanReport(context.Background(), input, _withAnalayzers(analyzer.TypNodeNpm))
	if err != nil || !r.Complete {
		t.Fatalf("%v %+v", err, r)
	}
	components := map[string]model.ComponentKey{}
	for _, c := range r.Components {
		components[c.Key.ID()] = c.Key
	}
	got := map[[2]string]record{}
	observations := map[string]model.Observation{}
	for _, o := range r.Observations {
		key := components[o.Component]
		if key.Ecosystem != "npm" {
			t.Fatalf("ecosystem %q", key.Ecosystem)
		}
		id := [2]string{o.File, o.NativeID}
		if _, exists := got[id]; exists {
			t.Fatalf("duplicate installation %v", id)
		}
		got[id] = record{key.Name, key.Version, key.Source, o.Kind}
		observations[o.ID()] = o
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("producer records differ:\nwant=%v\ngot=%v", want, got)
	}
	// Every producer constraint, including dev-only declarations, must survive
	// with its original scope and owning file. Never bind it to another project.
	for _, d := range declarations {
		found := 0
		for _, q := range r.Requirements {
			o := observations[q.From]
			if o.NativeID == d.Root && o.File == d.File && q.Target == d.Name && q.Constraint == d.Constraint && q.Scope == d.Scope {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("declaration count %d for %+v", found, d)
		}
	}
	for _, q := range r.Requirements {
		from, ok := observations[q.From]
		if !ok {
			t.Fatalf("missing source %q", q.From)
		}
		for _, id := range q.Resolved {
			to, ok := observations[id]
			if !ok || to.File != from.File {
				t.Fatalf("missing or cross-file target: %+v", q)
			}
		}
	}
}
