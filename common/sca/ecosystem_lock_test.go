package sca

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/lockjson"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
)

func TestEcosystemLockCorpora(t *testing.T) {
	for _, tc := range []struct {
		group, file string
		parser      types.Parser
	}{
		{"nuget_lock", "packages.lock.json", lockjson.Nuget{}},
		{"swift_resolved", "Package.resolved", lockjson.Swift{}},
	} {
		t.Run(tc.group, func(t *testing.T) {
			var oracle map[string][]struct {
				Name, Version, Source, Revision, Kind, Framework, ContentHash string
				Dependencies                                                  map[string]string
			}
			if err := json.Unmarshal(modernBytes(t, tc.group+"/upstream-oracle.json"), &oracle); err != nil {
				t.Fatal(err)
			}
			for file, rows := range oracle {
				t.Run(file, func(t *testing.T) {
					data := modernBytes(t, tc.group+"/"+file)
					libs, deps, err := tc.parser.Parse(nil, bytes.NewReader(data))
					if err != nil {
						t.Fatal(err)
					}
					report := modernScan(t, tc.group+"/"+file, tc.file)
					if len(libs) != len(rows) || len(report.Components) != len(rows) {
						t.Fatalf("inventory %d/%d want %d", len(libs), len(report.Components), len(rows))
					}
					found := map[string]types.Library{}
					for _, lib := range libs {
						found[lib.Name] = lib
					}
					for _, want := range rows {
						lib, ok := found[want.Name]
						if !ok || lib.Version != want.Version || lib.Source != want.Source || lib.Locations[0].StartLine < 1 {
							t.Fatalf("lost fields: %+v want %+v", lib, want)
						}
						if tc.group == "nuget_lock" {
							hash, e := base64.StdEncoding.DecodeString(want.ContentHash)
							if e != nil {
								t.Fatal(e)
							}
							if lib.Condition != want.Framework || lib.DeclaredIntegrity != want.ContentHash || lib.Verification != "sha512:"+hex.EncodeToString(hash) {
								t.Fatalf("lost NuGet fields %+v", lib)
							}
							for target, constraint := range want.Dependencies {
								matched := false
								for _, d := range deps {
									if d.ID == lib.ID {
										for _, q := range d.Requirements {
											if q.Target == target && q.Constraint == constraint && q.Resolved == found[strings.ToLower(target)].ID {
												matched = true
											}
										}
									}
								}
								if !matched {
									t.Fatalf("lost dependency %s -> %s", want.Name, target)
								}
							}
						} else {
							var variant []string
							if err := json.Unmarshal([]byte(lib.Variant), &variant); err != nil || len(variant) != 3 || variant[0] != want.Kind || variant[2] != want.Revision {
								t.Fatalf("lost Swift state %+v", lib)
							}
							if len(deps) != 0 || len(report.Requirements) != 0 {
								t.Fatal("invented Swift edges")
							}
						}
					}
				})
			}
		})
	}
}

func TestEcosystemLockSemantics(t *testing.T) {
	t.Run("NuGet-frameworks", func(t *testing.T) {
		r := modernScan(t, "nuget_lock/frameworks-v2.json", "src/packages.lock.json")
		if len(r.Components) != 6 || len(r.Observations) != 6 {
			t.Fatalf("lost framework records %+v", r.Components)
		}
		observations := map[string]model.Observation{}
		components := map[string]model.Component{}
		for _, c := range r.Components {
			components[c.Key.ID()] = c
			if c.Key.Ecosystem != "nuget" {
				t.Fatal(c)
			}
		}
		for _, o := range r.Observations {
			observations[o.ID()] = o
		}
		resolved, missing, projects := 0, 0, 0
		for _, q := range r.Requirements {
			if q.Condition == "" {
				continue
			} // original requested range declaration
			from := observations[q.From]
			if q.Condition != from.Condition {
				t.Fatal("lost framework condition", q)
			}
			for _, id := range q.Resolved {
				to := observations[id]
				if to.Condition != from.Condition {
					t.Fatal("cross-framework edge", q)
				}
				resolved++
			}
			if q.Target == "Absent" || q.Target == "Workspace" && q.Condition == "net8.0/win-x64" {
				missing++
				if len(q.Resolved) != 0 {
					t.Fatal("guessed absent target", q)
				}
			}
			assertSBOMReq(t, r, q.From, q.Target, q.Constraint, q.Condition, q.Resolved)
		}
		for _, o := range r.Observations {
			if o.Scope == "Project" {
				projects++
				c := components[o.Component]
				if o.Kind != "declared" || c.Key.Version != "" {
					t.Fatal("project claimed as restored package", o, c)
				}
			}
		}
		if resolved != 4 || missing != 2 || projects != 1 {
			t.Fatalf("edges=%d missing=%d projects=%d", resolved, missing, projects)
		}
		bom := dxtypes.CreateCycloneDXSBOMFromReport(r)
		hashes := 0
		for _, c := range bom.Components {
			for _, h := range c.Hashes {
				if h.Algorithm != "SHA-512" || len(h.Value) != 128 {
					t.Fatal(h)
				}
				hashes++
			}
		}
		if hashes != 5 {
			t.Fatalf("hashes %d", hashes)
		}
	})
	t.Run("Swift-states", func(t *testing.T) {
		r := modernScan(t, "swift_resolved/states-v3.json", "App.xcodeproj/project.xcworkspace/xcshareddata/swiftpm/Package.resolved")
		if len(r.Components) != 4 || len(r.Requirements) != 0 {
			t.Fatalf("pins/edges %+v", r)
		}
		for _, c := range r.Components {
			if c.Key.Ecosystem != "swift" || c.Key.Verification != "" {
				t.Fatal("revision mistaken for archive checksum", c)
			}
			if (c.Key.Name == "branch-lib" || c.Key.Name == "commit-lib") && c.Key.Version != "" {
				t.Fatal("fabricated release version", c)
			}
			if c.Key.Name == "commit-lib" && c.Key.Source != "/outside/snapshot/no-read" {
				t.Fatal("source lost")
			}
		}
		for _, o := range r.Observations {
			if o.Scope != "directness:unknown" || o.Kind != "locked" {
				t.Fatal(o)
			}
		}
	})
	// Each analyzer is independently selectable; no filename suffix matching.
	t.Run("selection", func(t *testing.T) {
		fs := fstest.MapFS{"Package.resolved": {Data: modernBytes(t, "swift_resolved/states-v3.json")}, "packages.lock.json": {Data: modernBytes(t, "nuget_lock/frameworks-v2.json")}, "packages.lock.json.bak": {Data: []byte("invalid")}}
		for _, tc := range []struct {
			typ   analyzer.TypAnalyzer
			count int
		}{{analyzer.TypNuget, 6}, {analyzer.TypSwift, 4}} {
			r, e := ScanReport(context.Background(), fs, _withAnalayzers(tc.typ))
			if e != nil || !r.Complete || len(r.Components) != tc.count {
				t.Fatalf("selection %s %v %+v", tc.typ, e, r)
			}
		}
	})
}

func TestEcosystemLockFailures(t *testing.T) {
	t.Run("NuGet-upstream-placeholder-hash", func(t *testing.T) {
		_, _, err := (lockjson.Nuget{}).Parse(nil, bytes.NewReader(modernBytes(t, "nuget_lock/upstream-invalid-hash.json")))
		if scanerr.CodeOf(err) != scanerr.MalformedInput {
			t.Fatal("invalid upstream placeholder promoted to SHA-512", err)
		}
	})

	for _, tc := range []struct{ group, file, seed string }{{"nuget_lock", "packages.lock.json", "frameworks-v2.json"}, {"swift_resolved", "Package.resolved", "states-v3.json"}} {
		for _, name := range []string{"future.json", "malformed.json"} {
			t.Run(tc.group+"/"+name, func(t *testing.T) {
				r, e := ScanReport(context.Background(), fstest.MapFS{tc.file: {Data: modernBytes(t, tc.group+"/"+name)}})
				want := scanerr.MalformedInput
				if name == "future.json" {
					want = scanerr.UnsupportedSyntax
				}
				if e == nil || r == nil || r.Complete {
					t.Fatal("invalid lock accepted", e, r)
				}
				found := false
				for _, d := range r.Diagnostics {
					if d.Code == want && d.Incomplete {
						found = true
					}
				}
				if !found {
					t.Fatal(r.Diagnostics)
				}
			})
		}
		t.Run(tc.group+"/budget-cancel", func(t *testing.T) {
			fs := fstest.MapFS{tc.file: {Data: modernBytes(t, tc.group+"/"+tc.seed)}}
			r, e := ScanReport(context.Background(), fs, WithResourceLimits(ResourceLimits{MaxResultBytes: 256}))
			if !errors.Is(e, scanerr.ErrResourceLimit) || r.Complete {
				t.Fatal(e, r)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, e = ScanReport(ctx, fs)
			if !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
		})
	}
}

func TestEcosystemLockRejectsAmbiguousShapes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		p     types.Parser
		input string
		code  string
	}{
		{"nuget-null", lockjson.Nuget{}, `{"version":1,"dependencies":null}`, scanerr.MalformedInput},
		{"nuget-null-group", lockjson.Nuget{}, `{"version":1,"dependencies":{"net8.0":null}}`, scanerr.MalformedInput},
		{"nuget-missing-version", lockjson.Nuget{}, `{"dependencies":{}}`, scanerr.MalformedInput},
		{"nuget-case-collision", lockjson.Nuget{}, `{"version":2,"dependencies":{"net8.0":{"A":{"type":"Direct","resolved":"1"},"a":{"type":"Transitive","resolved":"2"}}}}`, scanerr.MalformedInput},
		{"nuget-unknown-type", lockjson.Nuget{}, `{"version":2,"dependencies":{"net8.0":{"a":{"type":"NewType","resolved":"1"}}}}`, scanerr.UnsupportedSyntax},
		{"nuget-no-resolution", lockjson.Nuget{}, `{"version":2,"dependencies":{"net8.0":{"a":{"type":"Direct"}}}}`, scanerr.MalformedInput},
		{"nuget-bad-hash", lockjson.Nuget{}, `{"version":2,"dependencies":{"net8.0":{"a":{"type":"Direct","resolved":"1","contentHash":"TEMPLATE"}}}}`, scanerr.MalformedInput},
		{"nuget-dep-object", lockjson.Nuget{}, `{"version":2,"dependencies":{"net8.0":{"a":{"type":"Project","dependencies":{"b":{}}}}}}`, scanerr.MalformedInput},
		{"swift-null", lockjson.Swift{}, `{"version":3,"pins":null}`, scanerr.MalformedInput},
		{"swift-missing-location", lockjson.Swift{}, `{"version":3,"pins":[{"identity":"a","kind":"registry","state":{"version":"1.0.0"}}]}`, scanerr.MalformedInput},
		{"swift-empty-state", lockjson.Swift{}, `{"version":3,"pins":[{"identity":"a","kind":"registry","location":"","state":{}}]}`, scanerr.MalformedInput},
		{"swift-no-revision", lockjson.Swift{}, `{"version":2,"pins":[{"identity":"a","kind":"remoteSourceControl","location":"a.git","state":{"version":"1.0.0"}}]}`, scanerr.MalformedInput},
		{"swift-unknown-kind", lockjson.Swift{}, `{"version":3,"pins":[{"identity":"a","kind":"new","location":"","state":{"version":"1.0.0"}}]}`, scanerr.UnsupportedSyntax},
		{"swift-two-states", lockjson.Swift{}, `{"version":3,"pins":[{"identity":"a","kind":"remoteSourceControl","location":"a.git","state":{"version":"1.0.0","branch":"main","revision":"abcdef"}}]}`, scanerr.MalformedInput},
		{"swift-duplicate", lockjson.Swift{}, `{"version":2,"pins":[{"identity":"a","kind":"registry","location":"","state":{"version":"1"}},{"identity":"A","kind":"registry","location":"","state":{"version":"2"}}]}`, scanerr.MalformedInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, e := tc.p.Parse(nil, strings.NewReader(tc.input))
			if e == nil || scanerr.CodeOf(e) != tc.code {
				t.Fatalf("want %s got %v", tc.code, e)
			}
		})
	}
	for _, tc := range []struct {
		p   types.Parser
		raw string
	}{{lockjson.Nuget{}, `{"version":1,"dependencies":{}}`}, {lockjson.Swift{}, `{"version":1,"object":{"pins":[]}}`}, {lockjson.Swift{}, `{"version":2,"pins":[]}`}, {lockjson.Swift{}, `{"version":3,"pins":[]}`}} {
		a, b, e := tc.p.Parse(nil, strings.NewReader(tc.raw))
		if e != nil || len(a) != 0 || len(b) != 0 {
			t.Fatal(a, b, e)
		}
	}
}

func TestEcosystemLockIdentityAndCancellation(t *testing.T) {
	raw := []byte(`{"version":2,"dependencies":{"a\u0000b":{"C":{"type":"Project"}},"a":{"b\u0000C":{"type":"Project"}}}}`)
	libs, _, e := (lockjson.Nuget{}).Parse(nil, bytes.NewReader(raw))
	if e != nil || len(libs) != 2 || libs[0].ID == libs[1].ID {
		t.Fatal(libs, e)
	}
	for _, p := range []types.Parser{lockjson.Nuget{}, lockjson.Swift{}} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, e := p.Parse(nil, modernContextReader{Reader: bytes.NewReader(raw), ctx: ctx})
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	}
	// A long shared target name is charged for every native ID/output record,
	// rather than charging only the once-decoded map key.
	var buf strings.Builder
	buf.WriteString(`{"version":2,"dependencies":{"` + strings.Repeat("x", 1024) + `":{`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, _ := json.Marshal("pkg" + strconv.Itoa(i))
		buf.Write(key)
		buf.WriteString(`:{"type":"Project"}`)
	}
	buf.WriteString(`}}}`)
	limits, _ := (budget.Limits{MaxResultBytes: 2 << 20}).Normalize()
	_, _, e = (lockjson.Nuget{}).Parse(nil, modernContextReader{Reader: bytes.NewReader([]byte(buf.String())), ctx: budget.Bind(context.Background(), limits)})
	if !errors.Is(e, scanerr.ErrResourceLimit) {
		t.Fatal("fanout budget did not stop allocation", e)
	}
	limits, _ = (budget.Limits{MaxResultBytes: 8 << 20}).Normalize()
	got, _, e := (lockjson.Nuget{}).Parse(nil, modernContextReader{Reader: bytes.NewReader([]byte(buf.String())), ctx: budget.Bind(context.Background(), limits)})
	if e != nil || len(got) != 100 {
		t.Fatal("budget positive control", len(got), e)
	}

}

func FuzzEcosystemLocks(f *testing.F) {
	for _, raw := range []string{`{"version":2,"dependencies":{}}`, `{"version":3,"pins":[]}`, `{"version":2,"dependencies":{"net8.0":{"a":{"type":"Direct","resolved":"1.0.0"}}}}`, `{"version":3,"pins":[{"identity":"a","kind":"registry","location":"","state":{"version":"1.0.0"}}]}`} {
		f.Add([]byte(raw))
	}

	for _, name := range []string{"nuget_lock/frameworks-v2.json", "swift_resolved/states-v3.json"} {
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
		limits, _ := (budget.Limits{MaxResultBytes: 4 << 20, MaxExpressionNodes: 10000, MaxSyntaxDepth: 32}).Normalize()
		for _, p := range []types.Parser{lockjson.Nuget{}, lockjson.Swift{}} {
			a, d, e := p.Parse(nil, modernContextReader{Reader: bytes.NewReader(b), ctx: budget.Bind(context.Background(), limits)})
			if e != nil {
				continue
			}
			a2, d2, e2 := p.Parse(nil, modernContextReader{Reader: bytes.NewReader(b), ctx: budget.Bind(context.Background(), limits)})
			if e2 != nil || !reflect.DeepEqual(a, a2) || !reflect.DeepEqual(d, d2) {
				t.Fatal("nondeterministic successful parse")
			}
		}
	})
}
