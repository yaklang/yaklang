package sca

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
)

func gapScan(t *testing.T, files map[string]string, opts ...ScanOption) (*model.Report, error) {
	t.Helper()
	return ScanReport(context.Background(), extraFS(files), append([]ScanOption{WithSnapshotID("gap")}, opts...)...)
}

func TestReview5149Gap_UnnamedNPMRootIsNotAnonymousComponent(t *testing.T) {
	lockOnly := `{"lockfileVersion":3,"packages":{"":{"dependencies":{"demo":"^1.0.0"}},"node_modules/demo":{"version":"1.2.0"}}}`
	r, err := gapScan(t, map[string]string{"package-lock.json": lockOnly})
	if r == nil {
		t.Fatalf("nil report: %v", err)
	}
	for _, c := range r.Components {
		if c.Key.Name == "" {
			t.Fatalf("anonymous component: %s", extra5149JSON(t, r))
		}
	}
	found := false
	for _, q := range r.Requirements {
		if q.Target == "demo" && q.Constraint == "^1.0.0" && q.From == "" && len(q.Resolved) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("unnamed root declaration lost: %s", extra5149JSON(t, r))
	}
}

func TestReview5149Gap_NPMRootVariants(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		from  string
	}{
		{
			name:  "lock_named",
			files: map[string]string{"package-lock.json": `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0","dependencies":{"demo":"^1.0.0"}},"node_modules/demo":{"version":"1.2.0"}}}`},
			from:  "app",
		},
		{
			name:  "manifest_only",
			files: map[string]string{"package.json": `{"name":"app","version":"1.0.0","dependencies":{"demo":"^1.0.0"}}`},
			from:  "app",
		},
		{
			name: "both",
			files: map[string]string{
				"package.json":      `{"name":"app","version":"1.0.0","dependencies":{"demo":"^1.0.0"}}`,
				"package-lock.json": `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0","dependencies":{"demo":"^1.0.0"}},"node_modules/demo":{"version":"1.2.0"}}}`,
			},
			from: "app",
		},
		{
			name:  "workspace_root",
			files: map[string]string{"package-lock.json": `{"lockfileVersion":3,"packages":{"":{"name":"root","version":"1.0.0","workspaces":["packages/*"],"dependencies":{"demo":"^1.0.0"}},"packages/demo":{"name":"demo","version":"1.2.0"},"node_modules/demo":{"link":true,"resolved":"packages/demo"}}}`},
			from:  "root",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := gapScan(t, tc.files)
			if r == nil {
				t.Fatal(err)
			}
			for _, c := range r.Components {
				if c.Key.Name == "" {
					t.Fatal("anonymous component")
				}
			}
			names := map[string]string{}
			for _, c := range r.Components {
				names[c.Key.ID()] = c.Key.Name
			}
			obs := map[string]string{}
			for _, o := range r.Observations {
				obs[o.ID()] = names[o.Component]
			}
			ok := false
			for _, q := range r.Requirements {
				if q.Constraint != "^1.0.0" {
					continue
				}
				if q.Target != "demo" && !strings.Contains(q.Target, "demo") {
					continue
				}
				if obs[q.From] == tc.from || q.From == "" || tc.name == "manifest_only" {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("declaration/binding lost: %s", extra5149JSON(t, r))
			}
		})
	}
}

func TestReview5149Gap_TypedScanErrors(t *testing.T) {
	lock := `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0"},"node_modules/demo":{"version":"1.2.0"}}}`
	cases := []struct {
		name string
		run  func(t *testing.T) (*model.Report, error)
		code string
		is   error
	}{
		{
			name: "read_limit",
			run: func(t *testing.T) (*model.Report, error) {
				return gapScan(t, map[string]string{"package-lock.json": lock}, WithResourceLimits(ResourceLimits{MaxFileBytes: 8}))
			},
			code: scanerr.ResourceLimit,
			is:   scanerr.ErrResourceLimit,
		},
		{
			name: "cancelled",
			run: func(t *testing.T) (*model.Report, error) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ScanReport(ctx, extraFS(map[string]string{"package-lock.json": lock}))
			},
			code: scanerr.Cancelled,
			is:   scanerr.ErrCancelled,
		},
		{
			name: "invalid_path",
			run: func(t *testing.T) (*model.Report, error) {
				return gapScan(t, map[string]string{`a\b`: "x"})
			},
			code: scanerr.InvalidPath,
			is:   scanerr.ErrInvalidPath,
		},
		{
			name: "input_changed",
			run: func(t *testing.T) (*model.Report, error) {
				return ScanReport(context.Background(), growStatFS{MapFS: extraFS(map[string]string{"package-lock.json": lock})}, WithSnapshotID("gap"))
			},
			code: scanerr.InputChanged,
			is:   scanerr.ErrInputChanged,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := tc.run(t)
			if r == nil || err == nil || r.Complete {
				t.Fatalf("expected classified failure: report=%+v err=%v", r, err)
			}
			if !errors.Is(err, tc.is) {
				t.Fatalf("errors.Is(%v, %v)=false", err, tc.is)
			}
			var classified *scanerr.Error
			if !errors.As(err, &classified) || classified.Code != tc.code {
				t.Fatalf("errors.As lost type: %v", err)
			}
			seen := false
			for _, d := range r.Diagnostics {
				if d.Code == tc.code && d.Incomplete {
					seen = true
				}
			}
			if !seen {
				t.Fatalf("diagnostic code mismatch: %s", extra5149JSON(t, r))
			}
		})
	}
	t.Run("path_containing_code_text_is_not_resource_limit", func(t *testing.T) {
		r, err := gapScan(t, map[string]string{"package-lock.json": `{"lockfileVersion":3,"packages":{"node_modules/resource_limit":{"version":"1.0.0"}}}`})
		if err != nil {
			if errors.Is(err, scanerr.ErrResourceLimit) {
				t.Fatalf("filename text selected resource_limit: %v", err)
			}
			return
		}
		for _, d := range r.Diagnostics {
			if d.Code == scanerr.ResourceLimit {
				t.Fatalf("filename text became resource_limit: %s", extra5149JSON(t, r))
			}
		}
	})
}

type growStatFS struct{ fstest.MapFS }

func (g growStatFS) Open(name string) (fs.File, error) {
	f, err := g.MapFS.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &growStatFile{File: f, info: growStatInfo{FileInfo: info}}, nil
}

type growStatFile struct {
	fs.File
	info fs.FileInfo
}

func (g *growStatFile) Stat() (fs.FileInfo, error) { return g.info, nil }

type growStatInfo struct{ fs.FileInfo }

func (g growStatInfo) Size() int64 { return g.FileInfo.Size() + 1 }

func TestReview5149Gap_POMRequirementFields(t *testing.T) {
	t.Run("property_and_range_keep_raw_constraint", func(t *testing.T) {
		r, err := gapScan(t, map[string]string{
			"pom.xml": `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>app</artifactId><version>1.0</version><dependencies><dependency><groupId>org.review</groupId><artifactId>prop</artifactId><version>${missing}</version></dependency><dependency><groupId>org.review</groupId><artifactId>range</artifactId><version>[1,2)</version></dependency></dependencies></project>`,
		})
		if r == nil {
			t.Fatal(err)
		}
		got := map[string]model.Requirement{}
		names := map[string]string{}
		for _, c := range r.Components {
			names[c.Key.ID()] = c.Key.Name
		}
		obs := map[string]string{}
		for _, o := range r.Observations {
			obs[o.ID()] = names[o.Component]
		}
		for _, q := range r.Requirements {
			if obs[q.From] != "org.review:app" {
				continue
			}
			if q.Target == "org.review:prop" || q.Target == "org.review:range" {
				got[q.Target] = q
			}
		}
		if got["org.review:prop"].Constraint != "${missing}" {
			t.Fatalf("property constraint rewritten: %+v", got["org.review:prop"])
		}
		if len(got["org.review:prop"].Resolved) != 0 {
			t.Fatalf("unknown property marked resolved: %+v", got["org.review:prop"])
		}
		if got["org.review:range"].Constraint != "[1,2)" {
			t.Fatalf("range constraint rewritten: %+v", got["org.review:range"])
		}
		if len(got["org.review:range"].Resolved) != 0 {
			t.Fatalf("version range marked resolved: %+v", got["org.review:range"])
		}
	})
	t.Run("missing_bom_keeps_unresolved_declaration", func(t *testing.T) {
		r, err := gapScan(t, map[string]string{
			"pom.xml": `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>app</artifactId><version>1.0</version><dependencyManagement><dependencies><dependency><groupId>org.review</groupId><artifactId>missing-bom</artifactId><version>1.0</version><type>pom</type><scope>import</scope></dependency></dependencies></dependencyManagement><dependencies><dependency><groupId>org.review</groupId><artifactId>dep</artifactId></dependency></dependencies></project>`,
		})
		if r == nil {
			t.Fatal(err)
		}
		ok := false
		names := map[string]string{}
		for _, c := range r.Components {
			names[c.Key.ID()] = c.Key.Name
		}
		obs := map[string]string{}
		for _, o := range r.Observations {
			obs[o.ID()] = names[o.Component]
		}
		for _, q := range r.Requirements {
			if obs[q.From] != "org.review:app" || q.Target != "org.review:dep" {
				continue
			}
			if q.Constraint != "" {
				t.Fatalf("invented constraint: %+v", q)
			}
			if len(q.Resolved) != 0 {
				t.Fatalf("unknown version bound as resolved: %+v", q)
			}
			ok = true
		}
		if !ok {
			t.Fatalf("app->dep declaration missing: %s", extra5149JSON(t, r))
		}
	})
	t.Run("type_classifier_variants", func(t *testing.T) {
		r, err := gapScan(t, map[string]string{
			"pom.xml": `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>app</artifactId><version>1.0</version><dependencies><dependency><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version><type>jar</type></dependency><dependency><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version><type>jar</type><classifier>tests</classifier></dependency></dependencies></project>`,
			"repository/org/review/dep/1.0/dep-1.0.pom": `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version></project>`,
		})
		if r == nil {
			t.Fatal(err)
		}
		var jar, tests bool
		names := map[string]string{}
		for _, c := range r.Components {
			names[c.Key.ID()] = c.Key.Name
		}
		obs := map[string]string{}
		for _, o := range r.Observations {
			obs[o.ID()] = names[o.Component]
		}
		for _, q := range r.Requirements {
			if obs[q.From] != "org.review:app" {
				continue
			}
			if q.Target == "org.review:dep" && q.Constraint == "1.0" {
				jar = true
			}
			if strings.Contains(q.Target, "tests") && q.Constraint == "1.0" {
				tests = true
			}
		}
		if !jar || !tests {
			t.Fatalf("type/classifier variants collapsed: %s", extra5149JSON(t, r))
		}
	})
}

func TestReview5149Gap_DigestOriginalAndSBOM(t *testing.T) {
	sum1 := sha512.Sum512([]byte("one"))
	sum2 := sha512.Sum512([]byte("two"))
	b64 := base64.StdEncoding.EncodeToString(sum1[:])
	hexed := hex.EncodeToString(sum1[:])
	sri := "sha512-" + b64
	alt := "sha512:" + hexed
	sha1 := "sha1-" + base64.StdEncoding.EncodeToString(make([]byte, 20))
	lock := func(integrity string) string {
		return `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0","dependencies":{"demo":"1.2.0"}},"node_modules/demo":{"name":"demo","version":"1.2.0","resolved":"https://example.invalid/demo.tgz","integrity":"` + integrity + `"}}}`
	}
	t.Run("original_and_canonical", func(t *testing.T) {
		r, err := gapScan(t, map[string]string{"package-lock.json": lock(sri + " " + sha1)})
		if r == nil || err != nil {
			t.Fatalf("valid multi digest rejected: %v", err)
		}
		var ver string
		for _, c := range r.Components {
			if c.Key.Name == "demo" {
				ver = c.Key.Verification
			}
		}
		if !strings.Contains(extra5149JSON(t, r), sri) || !strings.Contains(extra5149JSON(t, r), sha1) {
			t.Fatalf("original integrity dropped: %s", extra5149JSON(t, r))
		}
		if !strings.Contains(ver, "sha512:") || !strings.Contains(ver, "sha1:") {
			t.Fatalf("canonical missing algorithms: %q", ver)
		}
		pkgs, err := ScanFilesystem(extraFS(map[string]string{"package-lock.json": lock(sri + " " + sha1)}), WithSnapshotID("gap"))
		if err != nil {
			t.Fatal(err)
		}
		fromReport := dxtypes.CreateCycloneDXSBOMFromReport(r)
		fromPkgs := dxtypes.CreateCycloneDXSBOMByDXPackages(pkgs)
		for i, bom := range []*dxtypes.BOM{fromReport, fromPkgs} {
			var hashes int
			var original bool
			for _, c := range bom.Components {
				if c.Name != "demo" {
					continue
				}
				hashes = len(c.Hashes)
				for _, p := range c.Properties {
					if p.Name == "sca:declared-integrity" && strings.Contains(p.Value, sri) {
						original = true
					}
				}
			}
			if hashes != 2 || !original {
				t.Fatalf("SBOM path %d lost digest evidence hashes=%d original=%v", i, hashes, original)
			}
		}
	})
	t.Run("encoding_variant_same_canonical", func(t *testing.T) {
		a, _ := gapScan(t, map[string]string{"package-lock.json": lock(sri)})
		b, _ := gapScan(t, map[string]string{"package-lock.json": lock(alt)})
		var va, vb string
		for _, c := range a.Components {
			if c.Key.Name == "demo" {
				va = c.Key.Verification
			}
		}
		for _, c := range b.Components {
			if c.Key.Name == "demo" {
				vb = c.Key.Verification
			}
		}
		if va == "" || va != vb {
			t.Fatalf("canonical diverged: %q %q", va, vb)
		}
		if !strings.Contains(extra5149JSON(t, a), sri) || !strings.Contains(extra5149JSON(t, b), alt) {
			t.Fatalf("original encodings not preserved")
		}
	})
	t.Run("unknown_and_illegal_not_hashes", func(t *testing.T) {
		r, err := gapScan(t, map[string]string{"package-lock.json": lock("sha999-abcd sha512-%%%% " + sri)})
		if r == nil {
			t.Fatal(err)
		}
		var ver string
		for _, c := range r.Components {
			if c.Key.Name == "demo" {
				ver = c.Key.Verification
			}
		}
		body := extra5149JSON(t, r)
		if !strings.Contains(body, "sha999-abcd") || !strings.Contains(body, "%%%%") {
			t.Fatalf("raw unknown/illegal dropped: %s", body)
		}
		if strings.Contains(ver, "sha999") || strings.Contains(ver, "%%%%") {
			t.Fatalf("illegal digest became identity: %q", ver)
		}
		if !strings.Contains(ver, "sha512:") {
			t.Fatalf("valid digest dropped: %q", ver)
		}
		bom := dxtypes.CreateCycloneDXSBOMFromReport(r)
		for _, c := range bom.Components {
			if c.Name != "demo" {
				continue
			}
			for _, h := range c.Hashes {
				if strings.Contains(strings.ToLower(h.Algorithm+h.Value), "999") || strings.Contains(h.Value, "%") {
					t.Fatalf("illegal hash in SBOM: %+v", h)
				}
			}
		}
	})
	t.Run("instance_conflict", func(t *testing.T) {
		second := "sha512-" + base64.StdEncoding.EncodeToString(sum2[:])
		raw := `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0","dependencies":{"demo":"1.2.0"}},"node_modules/demo":{"name":"demo","version":"1.2.0","resolved":"https://example.invalid/demo.tgz","integrity":"` + sri + `"},"node_modules/wrapper":{"name":"wrapper","version":"1.0.0","dependencies":{"demo":"1.2.0"}},"node_modules/wrapper/node_modules/demo":{"name":"demo","version":"1.2.0","resolved":"https://example.invalid/demo.tgz","integrity":"` + second + `"}}}`
		r, _ := gapScan(t, map[string]string{"package-lock.json": raw})
		vers := map[string]bool{}
		for _, c := range r.Components {
			if c.Key.Name == "demo" {
				vers[c.Key.Verification] = true
			}
		}
		body := extra5149JSON(t, r)
		if len(vers) != 2 || !strings.Contains(body, sri) || !strings.Contains(body, second) {
			t.Fatalf("conflict evidence lost vers=%v body=%s", vers, body)
		}
	})
}

func TestReview5149Gap_NonComponentLimitPermutation(t *testing.T) {
	mk := func() *dxtypes.Package {
		p := &dxtypes.Package{Name: "demo", Version: "1.0.0"}
		p.EnsureDetails()
		p.Ecosystem = "npm"
		p.Instance = "node_modules/demo"
		p.Snapshot = "fixed"
		p.ProjectRoot = "."
		return p
	}
	t.Run("observations", func(t *testing.T) {
		var baseline string
		for seed := 0; seed < 16; seed++ {
			p := mk()
			locs := []dxtypes.SourceRange{
				{StartLine: 1, EndLine: 1}, {StartLine: 2, EndLine: 2}, {StartLine: 3, EndLine: 3}, {StartLine: 4, EndLine: 4},
				{StartLine: 5, EndLine: 5}, {StartLine: 6, EndLine: 6}, {StartLine: 7, EndLine: 7}, {StartLine: 8, EndLine: 8},
			}
			for i := range locs {
				j := (i*3 + seed) % len(locs)
				locs[i], locs[j] = locs[j], locs[i]
			}
			p.Locations = locs
			p.StartLine, p.EndLine = locs[0].StartLine, locs[0].EndLine
			r := &model.Report{Complete: true}
			fillReport(r, []*dxtypes.Package{p}, ResourceLimits{MaxComponents: 100, MaxObservations: 3, MaxEdges: 1000})
			r.Normalize()
			got := extra5149JSON(t, r.Observations)
			if seed == 0 {
				baseline = got
			} else if got != baseline {
				t.Fatalf("observation truncation order-dependent\n%s\n%s", baseline, got)
			}
		}
	})
	t.Run("edges", func(t *testing.T) {
		var baseline string
		for seed := 0; seed < 16; seed++ {
			p := mk()
			var reqs []model.Requirement
			for i := 0; i < 8; i++ {
				reqs = append(reqs, model.Requirement{Target: fmt.Sprintf("t%d", i), Constraint: "^1.0.0", Scope: "runtime"})
			}
			for i := range reqs {
				j := (i*5 + seed) % len(reqs)
				reqs[i], reqs[j] = reqs[j], reqs[i]
			}
			p.Requirements = reqs
			r := &model.Report{Complete: true}
			fillReport(r, []*dxtypes.Package{p}, ResourceLimits{MaxComponents: 100, MaxObservations: 100, MaxEdges: 3})
			r.Normalize()
			got := extra5149JSON(t, r.Requirements)
			if seed == 0 {
				baseline = got
			} else if got != baseline {
				t.Fatalf("edge truncation order-dependent\n%s\n%s", baseline, got)
			}
		}
	})
	t.Run("same_identifier_locations", func(t *testing.T) {
		var baseline string
		for seed := 0; seed < 8; seed++ {
			a, b := mk(), mk()
			a.Locations = []dxtypes.SourceRange{{StartLine: 8, EndLine: 8}, {StartLine: 1, EndLine: 1}}
			b.Locations = []dxtypes.SourceRange{{StartLine: 3, EndLine: 3}, {StartLine: 2, EndLine: 2}}
			if seed%2 == 1 {
				a, b = b, a
			}
			r := &model.Report{Complete: true}
			fillReport(r, []*dxtypes.Package{a, b}, ResourceLimits{MaxComponents: 100, MaxObservations: 3, MaxEdges: 100})
			r.Normalize()
			got := extra5149JSON(t, r.Observations)
			if seed == 0 {
				baseline = got
			} else if got != baseline {
				t.Fatalf("same-identity location truncation order-dependent")
			}
		}
	})
}
