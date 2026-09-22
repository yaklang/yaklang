package sca

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/modernlock"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

func lockJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func lockLibraries(libs []types.Library) map[string]types.Library {
	m := map[string]types.Library{}
	for _, l := range libs {
		m[l.ID] = l
	}
	return m
}
func lockRequirements(ds []types.Dependency) map[string][]types.Requirement {
	m := map[string][]types.Requirement{}
	for _, d := range ds {
		m[d.ID] = d.Requirements
	}
	return m
}
func TestUVBunCorpora(t *testing.T) {
	t.Run("uv", func(t *testing.T) {
		var oracle map[string][]struct {
			Name, Version string
			Source        any
			Markers       []string
			Artifacts     []map[string]any
			Requirements  []struct {
				Name, Version, Scope, Marker string
				Source                       any
				Extra                        []string
			}
		}
		if e := json.Unmarshal(modernBytes(t, "uv_lock/upstream-oracle.json"), &oracle); e != nil {
			t.Fatal(e)
		}
		for file, rows := range oracle {
			t.Run(file, func(t *testing.T) {
				libs, ds, e := (modernlock.UV{}).Parse(nil, bytes.NewReader(modernBytes(t, "uv_lock/"+file)))
				if e != nil {
					t.Fatal(e)
				}
				r := modernScan(t, "uv_lock/"+file, "uv.lock")
				if len(libs) != len(rows) || len(r.Components) != len(rows) {
					t.Fatalf("inventory %d/%d want %d", len(libs), len(r.Components), len(rows))
				}
				byID := lockLibraries(libs)
				reqs := lockRequirements(ds)
				for _, want := range rows {
					id := lockJSON([]string{want.Name, want.Version, lockJSON(want.Source)})
					lib, ok := byID[id]
					if !ok || lib.Source != lockJSON(want.Source) || lib.Version != want.Version || lib.Locations[0].StartLine < 1 {
						t.Fatal("lost UV package", want.Name, lib)
					}
					var variant struct {
						Wheels            []map[string]any
						Sdist             map[string]any
						ResolutionMarkers []string `json:"resolution-markers"`
					}
					if e := json.Unmarshal([]byte(lib.Variant), &variant); e != nil {
						t.Fatal(e)
					}
					artifacts := variant.Wheels
					if variant.Sdist != nil {
						artifacts = append([]map[string]any{variant.Sdist}, artifacts...)
					}
					if len(artifacts) != len(want.Artifacts) {
						t.Fatal("lost artifact set", want.Name)
					}
					for i, a := range artifacts {
						if !reflect.DeepEqual(a, want.Artifacts[i]) {
							t.Fatal("artifact URL/hash association lost", want.Name)
						}
					}
					if len(reqs[id]) != len(want.Requirements) {
						t.Fatalf("%s requirements %d want %d", want.Name, len(reqs[id]), len(want.Requirements))
					}
					for _, q := range want.Requirements {
						matched := false
						for _, got := range reqs[id] {
							var c struct {
								Marker string
								Extra  []string
								Source string
							}
							if e := json.Unmarshal([]byte(got.Condition), &c); e != nil {
								t.Fatal(e)
							}
							if got.Target == q.Name && got.Constraint == q.Version && got.Scope == q.Scope && c.Marker == q.Marker && reflect.DeepEqual(c.Extra, q.Extra) {
								candidates := []string{}
								for _, candidate := range rows {
									if candidate.Name == q.Name && (q.Version == "" || candidate.Version == q.Version) && (q.Source == nil || lockJSON(candidate.Source) == lockJSON(q.Source)) {
										candidates = append(candidates, lockJSON([]string{candidate.Name, candidate.Version, lockJSON(candidate.Source)}))
									}
								}
								wantID := ""
								if len(candidates) == 1 {
									wantID = candidates[0]
								}
								if got.Resolved != wantID {
									t.Fatal("incorrect UV target", q, got)
								}
								matched = true
							}
						}
						if !matched {
							t.Fatal("lost UV declaration", q)
						}
					}
				}
			})
		}
	})
	t.Run("bun", func(t *testing.T) {
		var oracle map[string][]struct {
			Path, Name, Version, Source, Hash string
			Workspace                         bool
			Info                              struct{ Dependencies, DevDependencies, OptionalDependencies, PeerDependencies map[string]string }
		}
		if e := json.Unmarshal(modernBytes(t, "bun_lock/upstream-oracle.json"), &oracle); e != nil {
			t.Fatal(e)
		}
		for file, rows := range oracle {
			t.Run(file, func(t *testing.T) {
				libs, ds, e := (modernlock.Bun{}).Parse(nil, bytes.NewReader(modernBytes(t, "bun_lock/"+file)))
				if e != nil {
					t.Fatal(e)
				}
				r := modernScan(t, "bun_lock/"+file, "bun.lock")
				if len(libs) != len(rows) || len(r.Components) != len(rows) {
					t.Fatalf("inventory %d/%d want %d", len(libs), len(r.Components), len(rows))
				}
				byID := lockLibraries(libs)
				reqs := lockRequirements(ds)
				for _, want := range rows {
					kind := "package"
					source := want.Source
					if want.Workspace {
						kind = "workspace"
						source = "workspace:" + want.Path
					}
					id := lockJSON([]string{kind, want.Path})
					lib, ok := byID[id]
					if !ok || lib.Name != want.Name || lib.Version != want.Version || lib.Source != source || lib.DeclaredIntegrity != want.Hash || lib.Locations[0].StartLine < 1 {
						t.Fatal("lost Bun record", want, lib)
					}
					count := 0
					for _, g := range []struct {
						s string
						m map[string]string
					}{{"runtime", want.Info.Dependencies}, {"development", want.Info.DevDependencies}, {"optional", want.Info.OptionalDependencies}, {"peer", want.Info.PeerDependencies}} {
						for target, constraint := range g.m {
							count++
							found := false
							for _, q := range reqs[id] {
								if q.Target == target && q.Constraint == constraint && (q.Scope == g.s || g.s == "peer" && q.Scope == "optional-peer") {
									found = true
								}
							}
							if !found {
								t.Fatal("lost Bun declaration", want.Name, g.s, target)
							}
						}
					}
					if len(reqs[id]) != count {
						t.Fatal("extra Bun declarations", want.Name)
					}
				}
			})
		}
	})
}
func TestUVBunSemantics(t *testing.T) {
	t.Run("uv-branches", func(t *testing.T) {
		r := modernScan(t, "uv_lock/branches.lock", "uv.lock")
		if len(r.Components) != 7 {
			t.Fatal(len(r.Components))
		}
		libs, ds, e := (modernlock.UV{}).Parse(nil, bytes.NewReader(modernBytes(t, "uv_lock/branches.lock")))
		if e != nil {
			t.Fatal(e)
		}
		byID := lockLibraries(libs)
		resolved, missing, extras, groups := 0, 0, 0, 0
		for _, d := range ds {
			for _, q := range d.Requirements {
				if q.Resolved != "" {
					resolved++
					l := byID[q.Resolved]
					if q.Target != l.Name || q.Constraint != "" && q.Constraint != l.Version {
						t.Fatal(q, l)
					}
					if q.Target == "twin" && !strings.Contains(l.Source, "other.invalid") {
						t.Fatal("wrong registry", q)
					}
				} else {
					missing++
					if q.Target != "absent" && !(q.Target == "child" && q.Constraint == "") {
						t.Fatal(q)
					}
				}
				if strings.Contains(q.Condition, `"extra":["speed"]`) {
					extras++
				}
				if strings.Contains(q.Scope, "dependencies:") {
					groups++
				}
			}
		}
		if resolved != 6 || missing != 2 || extras != 1 || groups != 2 {
			t.Fatalf("resolved %d missing %d extras %d groups %d", resolved, missing, extras, groups)
		}
		for _, o := range r.Observations {
			if !strings.Contains(o.Condition, "conflicts") || !strings.Contains(o.Condition, "sys_platform") {
				t.Fatal("lost global environment/conflict evidence", o)
			}
		}
		for _, c := range r.Components {
			if c.Key.Ecosystem != "pypi" {
				t.Fatal(c)
			}
			if c.Key.Name == "local" && c.Key.Version != "" {
				t.Fatal("dynamic local invented version")
			}
		}
		for _, q := range r.Requirements {
			assertSBOMReq(t, r, q.From, q.Target, q.Constraint, q.Condition, q.Resolved)
		}
	})
	t.Run("bun-contexts", func(t *testing.T) {
		r := modernScan(t, "bun_lock/contexts-v2.lock", "bun.lock")
		if len(r.Components) != 12 {
			t.Fatal("inventory", len(r.Components))
		}
		libs, ds, e := (modernlock.Bun{}).Parse(nil, bytes.NewReader(modernBytes(t, "bun_lock/contexts-v2.lock")))
		if e != nil {
			t.Fatal(e)
		}
		byID := lockLibraries(libs)
		req := lockRequirements(ds)
		for _, tc := range []struct{ path, target, want string }{{"a", "child", "a/child"}, {"@scope/parent", "child", "@scope/parent/child"}, {"ws", "child", "ws/child"}, {"a", "missing", ""}, {"a", "peer", ""}} {
			found := false
			for _, q := range req[lockJSON([]string{"package", tc.path})] {
				if q.Target == tc.target {
					want := ""
					if tc.want != "" {
						want = lockJSON([]string{"package", tc.want})
					}
					if q.Resolved != want {
						t.Fatal("wrong installation target", tc, q)
					}
					found = true
				}
			}
			if !found {
				t.Fatal(tc)
			}
		}
		alias := byID[lockJSON([]string{"package", "alias"})]
		if alias.Name != "real" || alias.Version != "2.0.0" || alias.Source != "https://example.invalid/real.tgz" {
			t.Fatal(alias)
		}
		for _, q := range req[lockJSON([]string{"workspace", ""})] {
			if q.Target == "alias" && q.Resolved != alias.ID {
				t.Fatal("lost alias edge")
			}
		}
		for _, q := range req[lockJSON([]string{"workspace", "packages/ws"})] {
			if q.Resolved != "" {
				t.Fatal("uninstalled workspace borrowed root", q)
			}
		}
		for _, p := range []string{"git", "local"} {
			if byID[lockJSON([]string{"package", p})].Version != "" {
				t.Fatal("source fabricated version", p)
			}
		}
		for _, c := range r.Components {
			if c.Key.Ecosystem != "npm" {
				t.Fatal(c)
			}
		}
		for _, q := range r.Requirements {
			assertSBOMReq(t, r, q.From, q.Target, q.Constraint, q.Condition, q.Resolved)
		}
		modernScan(t, "bun_lock/workspace-v0.lock", "bun.lock")
		v3 := modernScan(t, "bun_lock/scoped-overrides-v3.lock", "bun.lock")
		for _, c := range v3.Components {
			var variant struct{ Declarations string }
			if e := json.Unmarshal([]byte(c.Key.Variant), &variant); e != nil {
				t.Fatal(e)
			}
			var declarations struct {
				Overrides           map[string]any
				PatchedDependencies map[string]string
			}
			if e := json.Unmarshal([]byte(variant.Declarations), &declarations); e != nil {
				t.Fatal(e)
			}
			if declarations.PatchedDependencies["child@2.0.0"] != "patches/child.patch" || lockJSON(declarations.Overrides["a"]) != `{"child":"1.0.0"}` {
				t.Fatal("lost override or patch provenance", c)
			}
		}
		// Overrides do not trigger another resolver: the installed nested child is 1.
		v3libs, v3deps, e := (modernlock.Bun{}).Parse(nil, bytes.NewReader(modernBytes(t, "bun_lock/scoped-overrides-v3.lock")))
		if e != nil {
			t.Fatal(e)
		}
		v3byID := lockLibraries(v3libs)
		for _, q := range lockRequirements(v3deps)[lockJSON([]string{"package", "a"})] {
			if q.Target == "child" && v3byID[q.Resolved].Version != "1.0.0" {
				t.Fatal("override graph lost", q)
			}
		}

	})
	t.Run("filter", func(t *testing.T) {
		fs := fstest.MapFS{"uv.lock": {Data: modernBytes(t, "uv_lock/branches.lock")}, "bun.lock": {Data: modernBytes(t, "bun_lock/contexts-v2.lock")}}
		for _, tc := range []struct {
			typ analyzer.TypAnalyzer
			n   int
		}{{analyzer.TypUV, 7}, {analyzer.TypBun, 12}} {
			r, e := ScanReport(context.Background(), fs, _withAnalayzers(tc.typ))
			if e != nil || len(r.Components) != tc.n {
				t.Fatal(tc, e)
			}
		}
	})
}
func TestUVBunFailures(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		p         types.Parser
		code      string
	}{
		{"uv-future", "version=1\nrevision=4\n", modernlock.UV{}, scanerr.UnsupportedSyntax},
		{"uv-future-major", "version=2\n", modernlock.UV{}, scanerr.UnsupportedSyntax},
		{"uv-ambiguous-source", "version=1\n[[package]]\nname='a'\nsource={git='a',registry='b'}", modernlock.UV{}, scanerr.MalformedInput},
		{"uv-duplicate", "version=1\n[[package]]\nname='A_b'\nsource={editable='.'}\n[[package]]\nname='a-b'\nsource={editable='.'}", modernlock.UV{}, scanerr.MalformedInput},
		{"uv-hash", "version=1\n[[package]]\nname='a'\nsource={editable='.'}\nwheels=[{hash='sha256:abc'}]", modernlock.UV{}, scanerr.MalformedInput},
		{"bun-null-patch", `{"lockfileVersion":3,"workspaces":{},"packages":{},"patchedDependencies":{"a@1":null}}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-bad-override", `{"lockfileVersion":3,"workspaces":{},"packages":{},"overrides":{"a":1}}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"uv-bad-conflict", "version=1\nconflicts=['a']\npackage=[]", modernlock.UV{}, scanerr.MalformedInput},
		{"bun-future", `{"lockfileVersion":4}`, modernlock.Bun{}, scanerr.UnsupportedSyntax},
		{"bun-null-version", `{"lockfileVersion":null}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-duplicate", `{"lockfileVersion":1,"lockfileVersion":2}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-null-package", `{"lockfileVersion":1,"workspaces":{},"packages":{"a":null}}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-leading-comma", `{"lockfileVersion":1,"workspaces":{},"packages":{"a":[,]}}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-double-comma", `{"lockfileVersion":1,"workspaces":{},"packages":{"a":["a@1",,]}}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-comment", `{"lockfileVersion":1} /*`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-config", `{"lockfileVersion":1,"configVersion":2}`, modernlock.Bun{}, scanerr.UnsupportedSyntax},
		{"bun-path", `{"lockfileVersion":1,"workspaces":{},"packages":{"../a":["a@1","",{},""]}}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-missing-workspace", `{"lockfileVersion":1,"workspaces":{},"packages":{"a":["a@workspace:foo"]}}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-missing-hash-v2", `{"lockfileVersion":2,"workspaces":{},"packages":{"a":["a@1.0.0","https://example.invalid/a.tgz",{},""]}}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-null-dep", `{"lockfileVersion":1,"workspaces":{},"packages":{"a":["a@1.0.0","",{"dependencies":{"b":null}},""]}}`, modernlock.Bun{}, scanerr.MalformedInput},
		{"bun-bundled", `{"lockfileVersion":1,"workspaces":{},"packages":{"a":["a@1.0.0","",{"bundledDependencies":["b"]},""]}}`, modernlock.Bun{}, scanerr.UnsupportedSyntax},
		{"bun-git-tag", `{"lockfileVersion":2,"workspaces":{},"packages":{"a":["a@github:a/b",{},"../escape"]}}`, modernlock.Bun{}, scanerr.MalformedInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, e := tc.p.Parse(nil, strings.NewReader(tc.raw))
			if e == nil || scanerr.CodeOf(e) != tc.code {
				t.Fatalf("want %s got %v", tc.code, e)
			}
		})
	}
	// Actual comments and commas in strings are not damaged by JSONC normalization.
	raw := "{\n// comment\n\"lockfileVersion\":1,/* block */\"workspaces\":{},\"packages\":{\"a\":[\"a@1.0.0\",\"https://example.invalid/a,}/*path*/\",{},\"\",],},}\n"
	a, _, e := (modernlock.Bun{}).Parse(nil, strings.NewReader(raw))
	if e != nil || len(a) != 1 || a[0].Source != "https://example.invalid/a,}/*path*/" || a[0].Locations[0].StartLine != 3 {
		t.Fatal(a, e)
	}
	for _, tc := range []struct {
		group, file string
		p           types.Parser
	}{{"uv_lock/branches.lock", "uv.lock", modernlock.UV{}}, {"bun_lock/contexts-v2.lock", "bun.lock", modernlock.Bun{}}} {
		raw := modernBytes(t, tc.group)
		fs := fstest.MapFS{tc.file: {Data: raw}}
		r, e := ScanReport(context.Background(), fs, WithResourceLimits(ResourceLimits{MaxResultBytes: 256}))
		if !errors.Is(e, scanerr.ErrResourceLimit) || r.Complete {
			t.Fatal("budget", e)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, e = tc.p.Parse(nil, modernContextReader{Reader: bytes.NewReader(raw), ctx: ctx})
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
		limits, _ := (budget.Limits{MaxResolveSteps: 1}).Normalize()
		_, _, e = tc.p.Parse(nil, modernContextReader{Reader: bytes.NewReader(raw), ctx: budget.Bind(context.Background(), limits)})
		if !errors.Is(e, scanerr.ErrResourceLimit) {
			t.Fatal("resolution budget", e)
		}
	}
}
func TestUVBunVersionAndSourceBoundaries(t *testing.T) {
	seed := modernBytes(t, "uv_lock/branches.lock")
	for _, rev := range []string{"0", "1", "2", "3"} {
		raw := bytes.Replace(seed, []byte("revision = 3"), []byte("revision = "+rev), 1)
		a, _, e := (modernlock.UV{}).Parse(nil, bytes.NewReader(raw))
		if e != nil || len(a) != 7 {
			t.Fatal(rev, e)
		}
	}
	for _, source := range []string{`{url="https://example.invalid/a.tar.gz",subdirectory="src"}`, `{path="../outside/a.whl"}`, `{directory="../outside"}`} {
		raw := "version=1\n[[package]]\nname='a'\nversion='1.0'\nsource=" + source
		libs, _, e := (modernlock.UV{}).Parse(nil, strings.NewReader(raw))
		if e != nil || len(libs) != 1 || libs[0].Source == "" {
			t.Fatal(source, e)
		}
	}
	for _, source := range []string{"file:../outside", "https://example.invalid/a.tgz"} {
		raw := `{"lockfileVersion":2,"workspaces":{},"packages":{"a":["a@` + source + `",{}]}}`
		libs, _, e := (modernlock.Bun{}).Parse(nil, strings.NewReader(raw))
		if e != nil || len(libs) != 1 || libs[0].Source != source || libs[0].Version != "" {
			t.Fatal(source, e)
		}
	}
	for _, raw := range []string{"version=1\n[[package]]\nname='-a'\nsource={editable='.'}", "version=1\n[[package]]\nname='a b'\nsource={editable='.'}"} {
		_, _, e := (modernlock.UV{}).Parse(nil, strings.NewReader(raw))
		if scanerr.CodeOf(e) != scanerr.MalformedInput {
			t.Fatal(e)
		}
	}
	_, _, e := (modernlock.Bun{}).Parse(nil, bytes.NewReader(append([]byte("//"), 0xff)))
	if scanerr.CodeOf(e) != scanerr.MalformedInput {
		t.Fatal("non UTF-8 comment", e)
	}
	// Workspace metadata is shared in the input but copied into every installed
	// occurrence's requirements. A small result budget must stop this fanout.
	workspace := map[string]any{"name": "w", "dependencies": map[string]string{strings.Repeat("a", 8192): "1"}}
	pkgs := map[string]any{}
	for i := 0; i < 100; i++ {
		pkgs[fmt.Sprintf("alias%d", i)] = []any{"w@workspace:w"}
	}
	raw := []byte(lockJSON(map[string]any{"lockfileVersion": 1, "workspaces": map[string]any{"w": workspace}, "packages": pkgs}))
	for _, limit := range []int64{2 << 20, 64 << 20} {
		a, _, e := (modernlock.Bun{}).Parse(nil, modernContextReader{Reader: bytes.NewReader(raw), ctx: budget.Bind(context.Background(), budget.Limits{MaxResultBytes: limit})})
		if limit == 2<<20 {
			if !errors.Is(e, scanerr.ErrResourceLimit) {
				t.Fatal("unbounded workspace fanout", e)
			}
		} else if e != nil || len(a) != 101 {
			t.Fatal("fanout positive control", len(a), e)
		}
	}
}

func FuzzUVBunLocks(f *testing.F) {
	for _, name := range []string{"uv_lock/branches.lock", "bun_lock/contexts-v2.lock", "bun_lock/upstream-v1-yaml.lock", "bun_lock/scoped-overrides-v3.lock"} {
		b, e := fixtures.ReadFile("testdata/modern/" + name)
		if e != nil {
			f.Fatal(e)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 128<<10 {
			t.Skip()
		}
		for _, p := range []types.Parser{modernlock.UV{}, modernlock.Bun{}} {
			parse := func() ([]types.Library, []types.Dependency, error) {
				return p.Parse(nil, modernContextReader{Reader: bytes.NewReader(b), ctx: budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 4 << 20, MaxExpressionNodes: 10000, MaxResolveSteps: 10000})})
			}
			a, d, e := parse()
			if e != nil {
				continue
			}
			a2, d2, e := parse()
			if e != nil || !reflect.DeepEqual(a, a2) || !reflect.DeepEqual(d, d2) {
				t.Fatal("nondeterminism", e)
			}
			ids := lockLibraries(a)
			if len(ids) != len(a) {
				t.Fatal("identity collision")
			}
			for _, dep := range d {
				for _, q := range dep.Requirements {
					if q.Resolved != "" {
						if _, ok := ids[q.Resolved]; !ok {
							t.Fatal("dangling edge")
						}
					}
				}
			}
		}
	})
}
