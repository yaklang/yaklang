package sca

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
)

type oldScanRec struct {
	Name, Version, Verification     string
	License                         []string
	Potential                       bool
	DependsAnd                      map[string]string
	UpNames, FromFile, FromAnalyzer []string
}

type origDep struct{ Target, Constraint, Raw string }

type origPkg struct {
	Name, Version, Verification, Source, Arch, LicenseRaw, Integrity, ResolvedURL string
	Provides                                                                      []string
	Depends                                                                       []origDep
	Start, End                                                                    int
	Dev                                                                           bool
	Indirect                                                                      bool
	DeclaredPath, DeclaredVersion, Variant                                        string
	Patterns                                                                      []string
}

func scanNamed(t *testing.T, id string, files map[string]string) *model.Report {
	t.Helper()
	in := fstest.MapFS{}
	for path, file := range files {
		raw, err := fixtures.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		in[path] = &fstest.MapFile{Data: raw}
	}
	r, err := ScanReport(context.Background(), in, WithSnapshotID("full-field-"+id))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	return r
}

func loadOldScan(t *testing.T, format string) []oldScanRec {
	t.Helper()
	raw, err := fixtures.ReadFile("testdata/full_field/old-scanfilesystem-" + format + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var out []oldScanRec
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty frozen old ScanFilesystem dump")
	}
	return out
}

func decodeApkC(value string) string {
	alg := "md5:"
	if strings.HasPrefix(value, "Q1") {
		alg = "sha1:"
		value = value[2:]
	}
	b, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return ""
	}
	return alg + hex.EncodeToString(b)
}

func parseApkInstalled(raw []byte) []origPkg {
	var out []origPkg
	for _, rec := range strings.Split(string(raw), "\n\n") {
		p := origPkg{}
		for _, line := range strings.Split(rec, "\n") {
			if len(line) < 2 {
				continue
			}
			switch line[:2] {
			case "P:":
				p.Name = line[2:]
			case "V:":
				p.Version = line[2:]
			case "A:":
				p.Arch = line[2:]
			case "L:":
				p.LicenseRaw = line[2:]
			case "C:":
				p.Verification = decodeApkC(line[2:])
			case "p:":
				p.Provides = strings.Fields(line[2:])
			case "D:":
				for _, tok := range strings.Fields(line[2:]) {
					if strings.HasPrefix(tok, "!") {
						continue
					}
					name, c := tok, "*"
					if i := strings.IndexAny(tok, "<>="); i >= 0 {
						name, c = tok[:i], tok[i:]
					}
					p.Depends = append(p.Depends, origDep{Target: name, Constraint: c, Raw: tok})
				}
			}
		}
		if p.Name != "" && p.Version != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseCargoLock(raw []byte) []origPkg {
	var out []origPkg
	var cur origPkg
	inDeps := false
	start := 0
	lines := strings.Split(string(raw), "\n")
	flush := func(end int) {
		if cur.Name != "" {
			for end > start && strings.TrimSpace(lines[end-1]) == "" {
				end--
			}
			cur.Start, cur.End = start, end
			out = append(out, cur)
		}
		cur, inDeps = origPkg{}, false
	}
	for i, line := range lines {
		n := i + 1
		trim := strings.TrimSpace(line)
		if trim == "[[package]]" {
			flush(n - 1)
			start = n
			continue
		}
		if inDeps {
			if trim == "]" {
				inDeps = false
				continue
			}
			dep := strings.Trim(trim, `",`)
			if dep == "" {
				continue
			}
			f := strings.Fields(dep)
			c := ""
			if len(f) > 1 {
				c = f[1]
			}
			cur.Depends = append(cur.Depends, origDep{f[0], c, dep})
			continue
		}
		switch {
		case strings.HasPrefix(trim, "name = "):
			cur.Name = strings.Trim(strings.TrimPrefix(trim, "name = "), `"`)
		case strings.HasPrefix(trim, "version = "):
			cur.Version = strings.Trim(strings.TrimPrefix(trim, "version = "), `"`)
		case strings.HasPrefix(trim, "source = "):
			cur.Source = strings.Trim(strings.TrimPrefix(trim, "source = "), `"`)
		case strings.HasPrefix(trim, "checksum = "):
			cur.Integrity = strings.Trim(strings.TrimPrefix(trim, "checksum = "), `"`)
			cur.Verification = "sha256:" + cur.Integrity
		case trim == "dependencies = [":
			inDeps = true
		}
	}
	flush(len(lines))
	return out
}

type npmLockV1 struct {
	Dependencies map[string]npmLockDep `json:"dependencies"`
}
type npmLockDep struct {
	Version      string                `json:"version"`
	Resolved     string                `json:"resolved"`
	Integrity    string                `json:"integrity"`
	Dev          bool                  `json:"dev"`
	Requires     map[string]string     `json:"requires"`
	Dependencies map[string]npmLockDep `json:"dependencies"`
}

func parseNpmLockV1(raw []byte) []origPkg {
	var lock npmLockV1
	if err := json.Unmarshal(raw, &lock); err != nil {
		return nil
	}
	var out []origPkg
	var walk func(map[string]npmLockDep)
	walk = func(m map[string]npmLockDep) {
		for name, d := range m {
			p := origPkg{Name: name, Version: d.Version, Integrity: d.Integrity, ResolvedURL: d.Resolved, Source: d.Resolved, Dev: d.Dev}
			if strings.HasPrefix(d.Integrity, "sha512-") {
				p.Verification = "sha512:" + hex.EncodeToString(mustB64(d.Integrity[len("sha512-"):]))
			} else if strings.HasPrefix(d.Integrity, "sha1-") {
				p.Verification = "sha1:" + hex.EncodeToString(mustB64(d.Integrity[len("sha1-"):]))
			}
			for tgt, c := range d.Requires {
				p.Depends = append(p.Depends, origDep{Target: tgt, Constraint: c, Raw: tgt + " " + c})
			}
			out = append(out, p)
			if len(d.Dependencies) > 0 {
				walk(d.Dependencies)
			}
		}
	}
	walk(lock.Dependencies)
	return out
}

func mustB64(s string) []byte {
	b, _ := base64.StdEncoding.DecodeString(s)
	return b
}

func parseGoMod(mod, sum []byte) []origPkg {
	type req struct {
		path, ver string
		indirect  bool
		line      int
	}
	var reqs []req
	repl := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(mod))
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(s, "require ") && !strings.HasPrefix(s, "require (") {
			f := strings.Fields(s)
			if len(f) >= 3 {
				reqs = append(reqs, req{f[1], f[2], strings.Contains(s, "// indirect"), line})
			}
		}
		if strings.HasPrefix(s, "replace ") {
			parts := strings.Split(s, "=>")
			if len(parts) == 2 {
				left := strings.Fields(parts[0])
				right := strings.Fields(parts[1])
				if len(left) >= 2 && len(right) >= 2 {
					repl[left[1]] = right[0] + " " + right[1]
				}
			}
		}
	}
	sums := map[string]string{}
	ss := bufio.NewScanner(bytes.NewReader(sum))
	for ss.Scan() {
		f := strings.Fields(ss.Text())
		if len(f) >= 3 && !strings.HasSuffix(f[1], "/go.mod") {
			sums[f[0]+" "+f[1]] = f[2]
		}
	}
	var out []origPkg
	for _, r := range reqs {
		p := origPkg{Name: r.path, Version: strings.TrimPrefix(r.ver, "v"), DeclaredPath: r.path, DeclaredVersion: r.ver, Start: r.line, End: r.line, Indirect: r.indirect}
		if nv, ok := repl[r.path]; ok {
			rf := strings.Fields(nv)
			p.Source, p.Version = rf[0], strings.TrimPrefix(rf[1], "v")
			p.Name = rf[0]
		}
		p.Verification = sums[p.Name+" v"+p.Version]
		out = append(out, p)
	}
	return out
}

func reportIndex(r *model.Report) (map[string]model.Component, map[string]model.Observation, map[string][]model.Observation) {
	comps := map[string]model.Component{}
	obs := map[string]model.Observation{}
	byComp := map[string][]model.Observation{}
	for _, c := range r.Components {
		comps[c.Key.Name+"@"+c.Key.Version] = c
	}
	for _, o := range r.Observations {
		obs[o.ID()] = o
		byComp[o.Component] = append(byComp[o.Component], o)
	}
	return comps, obs, byComp
}

func parseSBOM(t *testing.T, r *model.Report) (*dxtypes.BOM, []model.Requirement, []model.Observation) {
	t.Helper()
	bom := dxtypes.CreateCycloneDXSBOMFromReport(r)
	if bom == nil {
		t.Fatal("nil SBOM")
	}
	var reqs []model.Requirement
	var observations []model.Observation
	for _, p := range bom.Properties {
		switch p.Name {
		case "sca:requirements":
			if err := json.Unmarshal([]byte(p.Value), &reqs); err != nil {
				t.Fatal(err)
			}
		case "sca:observations":
			if err := json.Unmarshal([]byte(p.Value), &observations); err != nil {
				t.Fatal(err)
			}
		}
	}
	return bom, reqs, observations
}

func TestFourFormatFullFieldContract(t *testing.T) {
	t.Run("apk", testFullFieldAPK)
	t.Run("cargo", testFullFieldCargo)
	t.Run("npm", testFullFieldNPM)
	t.Run("gomod", testFullFieldGoMod)
	t.Run("yarn", testFullFieldYarn)
	t.Run("bundler", testFullFieldBundler)
	t.Run("pnpm", testFullFieldPnpm)
	t.Run("poetry", testFullFieldPoetry)
	t.Run("gemspec", testFullFieldGemspec)
	t.Run("packaging", testFullFieldPackaging)
	t.Run("pom", testFullFieldPOM)
	t.Run("gradle", testFullFieldGradle)
	t.Run("jar", testFullFieldJAR)
	t.Run("gobinary", testFullFieldGoBinary)
	t.Run("conan", testFullFieldConan)
	t.Run("dpkg", testFullFieldDPKG)
	t.Run("rpm", testFullFieldRPM)
	t.Run("pip", testFullFieldPip)
	t.Run("pipenv", testFullFieldPipenv)
	t.Run("composer", testFullFieldComposer)
}

func testFullFieldAPK(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/apk/apk")
	if err != nil {
		t.Fatal(err)
	}
	orig := parseApkInstalled(raw)
	old := loadOldScan(t, "apk")
	r := scanNamed(t, "apk", map[string]string{"lib/apk/db/installed": "testdata/apk/apk"})
	comps, obsByID, byComp := reportIndex(r)
	if len(orig) != len(r.Components) {
		t.Fatalf("installed packages %d components %d", len(orig), len(r.Components))
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		if !o.Potential {
			oldBy[o.Name+"@"+o.Version] = o
		}
	}
	for _, p := range orig {
		c, ok := comps[p.Name+"@"+p.Version]
		if !ok {
			t.Fatalf("missing component %s@%s", p.Name, p.Version)
		}
		if c.Key.Verification != p.Verification {
			t.Fatalf("%s verification got %q fixture %q", p.Name, c.Key.Verification, p.Verification)
		}
		if c.Key.Architecture != p.Arch {
			t.Fatalf("%s architecture got %q fixture A:%q", p.Name, c.Key.Architecture, p.Arch)
		}
		if c.Key.Source != "" {
			t.Fatalf("%s source is not an apk installed field, got %q", p.Name, c.Key.Source)
		}
		if c.Key.Ecosystem != "apk" {
			t.Fatalf("%s ecosystem %q", p.Name, c.Key.Ecosystem)
		}
		if strings.Join(c.Licenses, " ") != p.LicenseRaw {
			t.Fatalf("%s license got %q fixture L:%q", p.Name, c.Licenses, p.LicenseRaw)
		}
		oc := oldBy[p.Name+"@"+p.Version]
		if oc.Verification != p.Verification {
			t.Fatalf("old dump verification drifted from fixture for %s", p.Name)
		}
		hits := byComp[c.Key.ID()]
		if len(hits) == 0 {
			t.Fatalf("no observation for %s", p.Name)
		}
		o := hits[0]
		if o.Component != c.Key.ID() {
			t.Fatalf("observation component identity mismatch %s", p.Name)
		}
		if o.Kind != "installed" || o.File != "lib/apk/db/installed" {
			t.Fatalf("observation file/kind %s: %+v", p.Name, o)
		}
		if o.ID() == "" || obsByID[o.ID()].Component != c.Key.ID() {
			t.Fatalf("observation id not associated %s", p.Name)
		}
		gotProv := map[string]bool{}
		for _, s := range o.Provides {
			gotProv[s] = true
		}
		for _, s := range p.Provides {
			if !gotProv[s] {
				t.Fatalf("%s missing provide %q got %v", p.Name, s, o.Provides)
			}
		}
		for _, d := range p.Depends {
			found := false
			for _, q := range r.Requirements {
				if q.From != o.ID() || q.Target != d.Target {
					continue
				}
				found = true
				if q.Constraint != d.Constraint {
					t.Fatalf("%s depend %s constraint got %q fixture %q", p.Name, d.Target, q.Constraint, d.Constraint)
				}
				if q.Operator != "and" {
					t.Fatalf("%s depend operator %q", p.Name, q.Operator)
				}
			}
			if !found {
				t.Fatalf("%s missing original D: %s %s", p.Name, d.Target, d.Constraint)
			}
		}
		if oc.FromFile[0] != "lib/apk/db/installed" || oc.FromAnalyzer[0] != "apk-pkg" {
			t.Fatalf("old dump lost from identity: %+v", oc)
		}
	}
	bom, sbomReqs, sbomObs := parseSBOM(t, r)
	var alpine *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "alpine-baselayout" && bom.Components[i].Version == "3.4.3-r1" {
			alpine = &bom.Components[i]
		}
	}
	if alpine == nil {
		t.Fatal("SBOM missing alpine-baselayout")
	}
	if len(alpine.Hashes) == 0 || alpine.Hashes[0].Algorithm != "SHA-1" || alpine.Hashes[0].Value != "cf0bca32762cd5be9974f4c127467b0f93f78f20" {
		t.Fatalf("SBOM hash %+v", alpine.Hashes)
	}
	archOK := false
	for _, p := range alpine.Properties {
		if p.Name == "sca:architecture" && p.Value == "x86_64" {
			archOK = true
		}
	}
	if !archOK {
		t.Fatalf("SBOM architecture %+v", alpine.Properties)
	}
	c := comps["alpine-baselayout@3.4.3-r1"]
	from := byComp[c.Key.ID()][0].ID()
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "alpine-baselayout-data" && q.Constraint == "=3.4.3-r1" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM requirement not associated with alpine-baselayout observation: %s", extra5149JSON(t, sbomReqs))
	}
	if len(sbomObs) != len(r.Observations) {
		t.Fatalf("SBOM observations %d report %d", len(sbomObs), len(r.Observations))
	}
}

func testFullFieldCargo(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/rust_cargo/positive/Cargo.lock")
	if err != nil {
		t.Fatal(err)
	}
	orig := parseCargoLock(raw)
	old := loadOldScan(t, "cargo")
	r := scanNamed(t, "cargo", map[string]string{"Cargo.lock": "testdata/rust_cargo/positive/Cargo.lock"})
	comps, obsByID, byComp := reportIndex(r)
	if len(orig) != len(r.Components) || len(old) != len(r.Components) {
		t.Fatalf("cargo packages orig=%d old=%d new=%d", len(orig), len(old), len(r.Components))
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	for _, p := range orig {
		c, ok := comps[p.Name+"@"+p.Version]
		if !ok {
			t.Fatalf("missing %s@%s", p.Name, p.Version)
		}
		if c.Key.Verification != p.Verification {
			t.Fatalf("%s verification got %q fixture %q (old dump was empty)", p.Name, c.Key.Verification, p.Verification)
		}
		if c.Key.Source != p.Source {
			t.Fatalf("%s source got %q fixture %q", p.Name, c.Key.Source, p.Source)
		}
		if c.Key.Architecture != "" {
			t.Fatalf("%s architecture not in Cargo.lock, got %q", p.Name, c.Key.Architecture)
		}
		if len(c.Licenses) != 0 {
			t.Fatalf("%s license not in Cargo.lock, got %v", p.Name, c.Licenses)
		}
		oc := oldBy[p.Name+"@"+p.Version]
		if oc.Verification != "" {
			t.Fatalf("old cargo dump unexpectedly had verification for %s", p.Name)
		}
		hits := byComp[c.Key.ID()]
		if len(hits) == 0 {
			t.Fatalf("no observation for %s", p.Name)
		}
		o := hits[0]
		if o.Component != c.Key.ID() || o.File != "Cargo.lock" || o.Kind != "locked" {
			t.Fatalf("observation identity %+v", o)
		}
		if p.Start != 0 && (o.StartLine != p.Start || o.EndLine != p.End) {
			t.Fatalf("%s location got %d-%d fixture %d-%d", p.Name, o.StartLine, o.EndLine, p.Start, p.End)
		}
		if p.Integrity != "" && o.DeclaredIntegrity != p.Integrity {
			t.Fatalf("%s declaredIntegrity got %q fixture %q", p.Name, o.DeclaredIntegrity, p.Integrity)
		}
		if _, ok := obsByID[o.ID()]; !ok {
			t.Fatalf("observation id missing %s", p.Name)
		}
		for _, d := range p.Depends {
			found := false
			for _, q := range r.Requirements {
				if q.From != o.ID() || q.Target != d.Target {
					continue
				}
				found = true
				if strings.HasPrefix(q.Target, "unresolved-cargo:") {
					t.Fatalf("synthetic target for original %q: %+v", d.Raw, q)
				}
				if q.Constraint != d.Constraint {
					t.Fatalf("%s dep %s invented or dropped declared version: got %q fixture %q", p.Name, d.Target, q.Constraint, d.Constraint)
				}
				if q.Condition != d.Raw {
					t.Fatalf("%s dep raw form got %q fixture %q", p.Name, q.Condition, d.Raw)
				}
				if d.Constraint == "" {
					continue
				}
				if len(q.Resolved) != 1 {
					t.Fatalf("%s dep %s resolved %v", p.Name, d.Target, q.Resolved)
				}
				res := obsByID[q.Resolved[0]]
				want := comps[d.Target+"@"+d.Constraint]
				if res.Component != want.Key.ID() {
					t.Fatalf("%s dep %s resolved observation component mismatch", p.Name, d.Target)
				}
			}
			if !found {
				t.Fatalf("%s missing original lock dependency %s %s in %s", p.Name, d.Target, d.Constraint, extra5149JSON(t, r.Requirements))
			}
		}
	}
	bom, sbomReqs, _ := parseSBOM(t, r)
	var aho *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "aho-corasick" {
			aho = &bom.Components[i]
		}
	}
	if aho == nil || len(aho.Hashes) == 0 || aho.Hashes[0].Value != "cc936419f96fa211c1b9166887b38e5e40b19958e5b895be7c1f93adec7071ac" {
		t.Fatalf("SBOM aho-corasick hash %+v", aho)
	}
	app := comps["app@0.1.0"]
	from := byComp[app.Key.ID()][0].ID()
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "regex" && q.Constraint == "1.7.3" && q.Condition == "regex 1.7.3 (registry+https://github.com/rust-lang/crates.io-index)" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM app->regex original qualifier not associated: %s", extra5149JSON(t, sbomReqs))
	}
}

func testFullFieldNPM(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/node_npm/positive_folder/package-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	orig := parseNpmLockV1(raw)
	old := loadOldScan(t, "npm")
	r := scanNamed(t, "npm", map[string]string{"package-lock.json": "testdata/node_npm/positive_folder/package-lock.json"})
	comps, obsByID, byComp := reportIndex(r)
	uniq := map[string]origPkg{}
	for _, p := range orig {
		uniq[p.Name+"@"+p.Version] = p
	}
	if len(uniq) != len(r.Components) || len(old) != len(r.Components) {
		t.Fatalf("npm uniq=%d old=%d new=%d origWalk=%d", len(uniq), len(old), len(r.Components), len(orig))
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	for key, p := range uniq {
		c, ok := comps[key]
		if !ok {
			t.Fatalf("missing %s", key)
		}
		if p.Verification != "" && c.Key.Verification != p.Verification {
			t.Fatalf("%s verification got %q fixture %q", key, c.Key.Verification, p.Verification)
		}
		if c.Key.Source != p.ResolvedURL {
			t.Fatalf("%s source got %q fixture resolved %q", key, c.Key.Source, p.ResolvedURL)
		}
		if c.Key.Architecture != "" {
			t.Fatalf("%s architecture not in lock, got %q", key, c.Key.Architecture)
		}
		oc := oldBy[key]
		if oc.Verification != "" {
			t.Fatalf("old npm dump unexpectedly had verification for %s", key)
		}
		hits := byComp[c.Key.ID()]
		if len(hits) == 0 {
			t.Fatalf("no observation for %s", key)
		}
		for _, o := range hits {
			if o.Component != c.Key.ID() || o.File != "package-lock.json" || o.Kind != "locked" {
				t.Fatalf("observation identity %+v", o)
			}
			if p.Integrity != "" && o.DeclaredIntegrity != p.Integrity {
				t.Fatalf("%s declaredIntegrity got %q fixture %q", key, o.DeclaredIntegrity, p.Integrity)
			}
			if p.Dev && o.Scope != "dev" && len(hits) == 1 {
				t.Fatalf("%s dev scope lost: %+v", key, o)
			}
			if _, ok := obsByID[o.ID()]; !ok {
				t.Fatalf("observation id missing")
			}
		}
	}
	ansi := comps["ansi-colors@3.2.3"]
	if byComp[ansi.Key.ID()][0].Scope != "dev" {
		t.Fatalf("ansi-colors must keep lock dev scope: %+v", byComp[ansi.Key.ID()])
	}
	body := comps["body-parser@1.18.3"]
	from := byComp[body.Key.ID()][0].ID()
	found := false
	for _, q := range r.Requirements {
		if q.From == from && q.Target == "debug" && q.Constraint == "2.6.9" {
			found = true
			if len(q.Resolved) != 1 {
				t.Fatalf("body-parser debug resolved %v", q.Resolved)
			}
			res := obsByID[q.Resolved[0]]
			dbg := comps["debug@2.6.9"]
			if res.Component != dbg.Key.ID() {
				t.Fatalf("resolved debug observation is not debug@2.6.9")
			}
		}
	}
	if !found {
		t.Fatalf("body-parser -> debug 2.6.9 not associated: %s", extra5149JSON(t, r.Requirements))
	}
	bom, sbomReqs, _ := parseSBOM(t, r)
	var ac *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "ansi-colors" {
			ac = &bom.Components[i]
		}
	}
	if ac == nil || len(ac.Hashes) == 0 || ac.Hashes[0].Algorithm != "SHA-512" {
		t.Fatalf("SBOM ansi-colors hash %+v", ac)
	}
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "debug" && q.Constraint == "2.6.9" {
			ok = true
		}
	}
	if !ok {
		t.Fatal("SBOM lost body-parser debug association")
	}
}

func testFullFieldGoMod(t *testing.T) {
	mod, err := fixtures.ReadFile("testdata/go_mod/positive/mod")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := fixtures.ReadFile("testdata/go_mod/positive/sum")
	if err != nil {
		t.Fatal(err)
	}
	orig := parseGoMod(mod, sum)
	old := loadOldScan(t, "gomod")
	r := scanNamed(t, "gomod", map[string]string{"go.mod": "testdata/go_mod/positive/mod", "go.sum": "testdata/go_mod/positive/sum"})
	comps, obsByID, byComp := reportIndex(r)
	if len(orig) != 2 || len(old) != 2 || len(r.Components) != 2 {
		t.Fatalf("gomod orig=%d old=%d new=%d", len(orig), len(old), len(r.Components))
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	for _, p := range orig {
		c, ok := comps[p.Name+"@"+p.Version]
		if !ok {
			t.Fatalf("missing %s@%s", p.Name, p.Version)
		}
		if c.Key.Verification != p.Verification {
			t.Fatalf("%s verification got %q fixture sum %q", p.Name, c.Key.Verification, p.Verification)
		}
		if p.Source != "" && c.Key.Source != p.Source {
			t.Fatalf("%s source got %q fixture replace %q", p.Name, c.Key.Source, p.Source)
		}
		if c.Key.Ecosystem != "golang" {
			t.Fatalf("%s ecosystem %q", p.Name, c.Key.Ecosystem)
		}
		if c.Key.Architecture != "" || len(c.Licenses) != 0 {
			t.Fatalf("%s invented arch/license: %+v", p.Name, c)
		}
		oc := oldBy[p.Name+"@"+p.Version]
		if oc.Verification != "" {
			t.Fatalf("old gomod dump unexpectedly had verification for %s", p.Name)
		}
		hits := byComp[c.Key.ID()]
		if len(hits) != 1 {
			t.Fatalf("observations for %s: %+v", p.Name, hits)
		}
		o := hits[0]
		if o.Component != c.Key.ID() || o.File != "go.mod" || o.Kind != "declared" {
			t.Fatalf("observation %+v", o)
		}
		if o.StartLine != p.Start || o.EndLine != p.End {
			t.Fatalf("%s location got %d-%d fixture line %d", p.Name, o.StartLine, o.EndLine, p.Start)
		}
		if _, ok := obsByID[o.ID()]; !ok {
			t.Fatal("observation id missing")
		}
		found := false
		for _, q := range r.Requirements {
			if q.From != o.ID() || q.Target != p.DeclaredPath {
				continue
			}
			found = true
			if q.Constraint != p.DeclaredVersion {
				t.Fatalf("%s declared constraint got %q fixture %q", p.Name, q.Constraint, p.DeclaredVersion)
			}
			if p.Indirect && q.Scope != "indirect" {
				t.Fatalf("%s indirect scope got %q", p.Name, q.Scope)
			}
			if !p.Indirect && q.Scope != "declaration" {
				t.Fatalf("%s declaration scope got %q", p.Name, q.Scope)
			}
		}
		if !found {
			t.Fatalf("missing associated declaration requirement for %s: %s", p.Name, extra5149JSON(t, r.Requirements))
		}
	}
	bom, sbomReqs, _ := parseSBOM(t, r)
	var xe *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "golang.org/x/xerrors" {
			xe = &bom.Components[i]
		}
	}
	if xe == nil {
		t.Fatal("SBOM missing xerrors")
	}
	verOK := false
	for _, p := range xe.Properties {
		if p.Name == "sca:verification" && p.Value == "h1:go1bK/D/BFZV2I8cIQd1NKEZ+0owSTG1fDTci4IqFcE=" {
			verOK = true
		}
	}
	if !verOK {
		t.Fatalf("SBOM xerrors verification %+v hashes %+v", xe.Properties, xe.Hashes)
	}
	dep := comps["github.com/aquasecurity/go-dep-parser@0.0.0-20220406074731-71021a481237"]
	from := byComp[dep.Key.ID()][0].ID()
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "github.com/aquasecurity/go-dep-parser" && q.Constraint == "v0.0.0-20211110174639-8257534ffed3" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM lost original require version: %s", extra5149JSON(t, sbomReqs))
	}
}

func yarnDescriptorPatterns(hdr string) []string {
	hdr = strings.TrimSuffix(strings.TrimSpace(hdr), ":")
	hdr = strings.Trim(hdr, `"`)
	var out []string
	for _, part := range strings.Split(hdr, ", ") {
		part = strings.Trim(part, `"`)
		name := yarnHeaderName(part)
		if name == "" {
			continue
		}
		rest := strings.Trim(strings.TrimPrefix(part, name+"@"), `"`)
		out = append(out, name+"@"+rest)
	}
	return out
}

func yarnHeaderName(hdr string) string {
	hdr = strings.TrimSuffix(strings.Trim(strings.TrimSpace(hdr), `"`), ":")
	first := strings.Split(hdr, ", ")[0]
	if strings.HasPrefix(first, "@") {
		i := strings.LastIndex(first[1:], "@")
		if i >= 0 {
			return first[:i+1]
		}
		return first
	}
	i := strings.LastIndex(first, "@")
	if i > 0 {
		return first[:i]
	}
	return first
}

func sriCanonical(integrity string) string {
	alg, b64, ok := strings.Cut(integrity, "-")
	if !ok {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ""
	}
	switch strings.ToLower(alg) {
	case "sha512":
		return "sha512:" + hex.EncodeToString(raw)
	case "sha1":
		return "sha1:" + hex.EncodeToString(raw)
	case "sha256":
		return "sha256:" + hex.EncodeToString(raw)
	}
	return ""
}

func parseYarnLock(raw []byte) []origPkg {
	var out []origPkg
	var cur origPkg
	inDeps := false
	line := 0
	flush := func() {
		if cur.Name != "" && cur.Version != "" {
			out = append(out, cur)
		}
		cur, inDeps = origPkg{}, false
	}
	for _, ln := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line++
		trim := strings.TrimSpace(ln)
		if trim == "" {
			if cur.Name != "" {
				flush()
			}
			continue
		}
		if strings.HasPrefix(trim, "#") {
			continue
		}
		if !strings.HasPrefix(ln, " ") && strings.Contains(trim, "@") && !strings.HasPrefix(trim, "version") && !strings.HasPrefix(trim, "resolved") {
			flush()
			cur = origPkg{Name: yarnHeaderName(trim), Start: line, End: line, Patterns: yarnDescriptorPatterns(ln)}
			inDeps = false
			continue
		}
		if cur.Name != "" {
			cur.End = line
		}
		if inDeps {
			if strings.HasPrefix(ln, "    ") && !strings.HasSuffix(trim, ":") {
				rest := strings.TrimSpace(ln)
				name, ver := "", ""
				if i := strings.Index(rest, " "); i > 0 {
					name = strings.Trim(rest[:i], `"`)
					ver = strings.Trim(rest[i+1:], `"`)
				} else {
					name = strings.Trim(rest, `"`)
				}
				if name != "" {
					rawDep := name
					if ver != "" {
						rawDep = name + "@" + ver
					}
					cur.Depends = append(cur.Depends, origDep{Target: name, Constraint: ver, Raw: rawDep})
				}
				continue
			}
			inDeps = false
		}
		switch {
		case strings.HasPrefix(trim, "version "):
			cur.Version = strings.Trim(strings.TrimPrefix(trim, "version "), `"`)
		case strings.HasPrefix(trim, "resolved "):
			cur.Source = strings.Trim(strings.TrimPrefix(trim, "resolved "), `"`)
		case strings.HasPrefix(trim, "integrity "):
			cur.Integrity = strings.Trim(strings.TrimPrefix(trim, "integrity "), `"`)
			cur.Verification = sriCanonical(cur.Integrity)
		case trim == "dependencies:" || trim == "optionalDependencies:":
			inDeps = true
		}
	}
	flush()
	return out
}

func parseBundlerLock(raw []byte) []origPkg {
	var out []origPkg
	section, source, revision := "", "", ""
	var cur *origPkg
	line := 0
	for _, rawLine := range strings.Split(string(raw), "\n") {
		line++
		text := strings.TrimSpace(rawLine)
		if text == "" {
			continue
		}
		indent := len(rawLine) - len(strings.TrimLeft(rawLine, " "))
		if indent == 0 {
			section = text
			source, revision = "", ""
			cur = nil
			continue
		}
		if section != "GEM" && section != "GIT" && section != "PATH" {
			continue
		}
		if indent == 2 {
			if strings.HasPrefix(text, "remote:") {
				source = strings.TrimSpace(strings.TrimPrefix(text, "remote:"))
			}
			if strings.HasPrefix(text, "revision:") {
				revision = strings.TrimSpace(strings.TrimPrefix(text, "revision:"))
			}
			continue
		}
		if indent == 4 {
			i := strings.Index(text, " (")
			if i < 1 || !strings.HasSuffix(text, ")") {
				continue
			}
			name := text[:i]
			version := text[i+2 : len(text)-1]
			variant := ""
			for _, suffix := range []string{"-x86_64", "-x86-", "-arm64", "-aarch64", "-universal", "-java", "-mingw", "-mswin"} {
				if j := strings.Index(version, suffix); j >= 0 {
					variant = version[j+1:]
					version = version[:j]
					break
				}
			}
			origin := source
			if revision != "" {
				origin += "#" + revision
			}
			p := origPkg{Name: name, Version: version, Source: origin, Variant: variant, Start: line, End: line}
			out = append(out, p)
			cur = &out[len(out)-1]
			continue
		}
		if indent == 6 && cur != nil {
			fields := strings.Fields(text)
			name := fields[0]
			constraint := strings.TrimSpace(strings.TrimPrefix(text, name))
			constraint = strings.TrimSuffix(strings.TrimPrefix(constraint, "("), ")")
			cur.Depends = append(cur.Depends, origDep{Target: name, Constraint: constraint, Raw: text})
		}
	}
	return out
}

func testFullFieldYarn(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/node_yarn/positive/yarn.lock")
	if err != nil {
		t.Fatal(err)
	}
	orig := parseYarnLock(raw)
	old := loadOldScan(t, "yarn")
	r := scanNamed(t, "yarn", map[string]string{"yarn.lock": "testdata/node_yarn/positive/yarn.lock"})
	comps, obsByID, byComp := reportIndex(r)
	if len(orig) != len(r.Components) || len(old) != len(r.Components) {
		t.Fatalf("yarn orig=%d old=%d new=%d", len(orig), len(old), len(r.Components))
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	patternOwner := map[string]origPkg{}
	for _, p := range orig {
		for _, pat := range p.Patterns {
			patternOwner[pat] = p
		}
	}
	for _, p := range orig {
		c, ok := comps[p.Name+"@"+p.Version]
		if !ok {
			t.Fatalf("missing %s@%s", p.Name, p.Version)
		}
		if c.Key.Verification != p.Verification {
			t.Fatalf("%s verification got %q fixture %q", p.Name, c.Key.Verification, p.Verification)
		}
		if c.Key.Source != p.Source {
			t.Fatalf("%s source got %q fixture %q", p.Name, c.Key.Source, p.Source)
		}
		if c.Key.Architecture != "" || len(c.Licenses) != 0 {
			t.Fatalf("%s invented arch/license %+v", p.Name, c)
		}
		if c.Key.Ecosystem != "npm" {
			t.Fatalf("%s ecosystem %q", p.Name, c.Key.Ecosystem)
		}
		oc := oldBy[p.Name+"@"+p.Version]
		if oc.Verification != "" {
			t.Fatalf("old yarn dump had verification for %s", p.Name)
		}
		if len(oc.FromFile) == 0 || oc.FromFile[0] != "yarn.lock" || len(oc.FromAnalyzer) == 0 || oc.FromAnalyzer[0] != "yarm-lang" {
			t.Fatalf("old dump lost from identity: %+v", oc)
		}
		for _, up := range oc.UpNames {
			at := strings.LastIndex(up, "@")
			if at < 1 {
				t.Fatalf("old UpNames not locked name@version: %q", up)
			}
			if _, ok := comps[up]; !ok {
				t.Fatalf("old dump UpNames %q is not a locked identity in fixture", up)
			}
		}
		hits := byComp[c.Key.ID()]
		if len(hits) == 0 {
			t.Fatalf("no observation for %s", p.Name)
		}
		o := hits[0]
		if o.Component != c.Key.ID() || o.File != "yarn.lock" || o.Kind != "locked" {
			t.Fatalf("observation %+v", o)
		}
		if p.Integrity != "" && o.DeclaredIntegrity != p.Integrity {
			t.Fatalf("%s declaredIntegrity got %q fixture %q", p.Name, o.DeclaredIntegrity, p.Integrity)
		}
		if p.Start != 0 && (o.StartLine != p.Start || o.EndLine != p.End) {
			t.Fatalf("%s location got %d-%d fixture %d-%d", p.Name, o.StartLine, o.EndLine, p.Start, p.End)
		}
		if _, ok := obsByID[o.ID()]; !ok {
			t.Fatal("observation id missing")
		}
		for _, d := range p.Depends {
			found := false
			for _, q := range r.Requirements {
				if q.From != o.ID() || q.Target != d.Target || q.Constraint != d.Constraint {
					continue
				}
				found = true
				if strings.HasPrefix(q.Target, "unresolved-yarn:") || strings.HasPrefix(q.Target, "[") {
					t.Fatalf("synthetic/native target %+v", q)
				}
				if q.Condition != d.Raw {
					t.Fatalf("%s dep raw got %q fixture %q", p.Name, q.Condition, d.Raw)
				}
				if d.Constraint != c.Key.Version && q.Constraint == c.Key.Version {
					t.Fatalf("locked version used as original range")
				}
				owner, exact := patternOwner[d.Raw]
				if exact {
					if len(q.Resolved) != 1 {
						t.Fatalf("%s dep %s must resolve exact descriptor %s: %v", p.Name, d.Target, d.Raw, q.Resolved)
					}
					want := comps[owner.Name+"@"+owner.Version]
					if obsByID[q.Resolved[0]].Component != want.Key.ID() {
						t.Fatalf("%s dep %s resolved to wrong locked identity, want %s@%s", p.Name, d.Target, owner.Name, owner.Version)
					}
				} else if len(q.Resolved) != 0 {
					t.Fatalf("%s dep %s unmatched descriptor uniquely resolved by order: %+v", p.Name, d.Raw, q)
				}
			}
			if !found {
				t.Fatalf("%s missing original yarn dep %s %s: %s", p.Name, d.Target, d.Constraint, extra5149JSON(t, r.Requirements))
			}
		}
	}
	bom, sbomReqs, _ := parseSBOM(t, r)
	var le *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "loose-envify" {
			le = &bom.Components[i]
		}
	}
	if le == nil || len(le.Hashes) == 0 {
		t.Fatalf("SBOM loose-envify %+v", le)
	}
	loose := comps["loose-envify@1.4.0"]
	from := byComp[loose.Key.ID()][0].ID()
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "js-tokens" && q.Constraint == "^3.0.0 || ^4.0.0" {
			ok = true
			if len(q.Resolved) != 1 {
				t.Fatalf("SBOM resolved %v", q.Resolved)
			}
			js4 := comps["js-tokens@4.0.0"]
			res := obsByID[q.Resolved[0]]
			if res.Component != js4.Key.ID() {
				t.Fatalf("SBOM resolved wrong js-tokens")
			}
		}
	}
	if !ok {
		t.Fatalf("SBOM lost original yarn range: %s", extra5149JSON(t, sbomReqs))
	}
}

func testFullFieldBundler(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/ruby_bundler/positive/Gemfile.lock")
	if err != nil {
		t.Fatal(err)
	}
	orig := parseBundlerLock(raw)
	old := loadOldScan(t, "bundler")
	r := scanNamed(t, "bundler", map[string]string{"Gemfile.lock": "testdata/ruby_bundler/positive/Gemfile.lock"})
	comps, obsByID, byComp := reportIndex(r)
	if len(orig) != len(r.Components) || len(old) != len(r.Components) {
		t.Fatalf("bundler orig=%d old=%d new=%d", len(orig), len(old), len(r.Components))
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	nameCount := map[string]int{}
	for _, c := range r.Components {
		nameCount[c.Key.Name]++
	}
	byName := map[string]model.Component{}
	for _, c := range r.Components {
		if nameCount[c.Key.Name] == 1 {
			byName[c.Key.Name] = c
		}
	}
	for _, p := range orig {
		c, ok := comps[p.Name+"@"+p.Version]
		if !ok {
			t.Fatalf("missing %s@%s", p.Name, p.Version)
		}
		if c.Key.Source != p.Source {
			t.Fatalf("%s source got %q fixture %q", p.Name, c.Key.Source, p.Source)
		}
		if c.Key.Architecture != "" {
			t.Fatalf("%s architecture not a Gemfile.lock field: %q", p.Name, c.Key.Architecture)
		}
		if p.Variant != "" && c.Key.Variant != p.Variant {
			t.Fatalf("%s platform variant got %q fixture %q", p.Name, c.Key.Variant, p.Variant)
		}
		if len(c.Licenses) != 0 || c.Key.Verification != "" {
			t.Fatalf("%s invented license/verification %+v", p.Name, c)
		}
		oc := oldBy[p.Name+"@"+p.Version]
		if oc.Name == "" {
			t.Fatalf("old dump missing %s@%s", p.Name, p.Version)
		}
		if len(oc.FromFile) == 0 || oc.FromFile[0] != "Gemfile.lock" || len(oc.FromAnalyzer) == 0 || oc.FromAnalyzer[0] != "ruby-bundler-lang" {
			t.Fatalf("old dump lost from identity: %+v", oc)
		}
		for _, up := range oc.UpNames {
			at := strings.LastIndex(up, "@")
			if at < 1 {
				t.Fatalf("old UpNames not locked name@version: %q", up)
			}
			if _, ok := comps[up]; !ok {
				t.Fatalf("old dump UpNames %q is not a locked identity in fixture", up)
			}
		}
		hits := byComp[c.Key.ID()]
		if len(hits) == 0 {
			t.Fatalf("no observation for %s", p.Name)
		}
		o := hits[0]
		if o.Component != c.Key.ID() || o.File != "Gemfile.lock" || o.Kind != "locked" {
			t.Fatalf("observation %+v", o)
		}
		if o.StartLine != p.Start {
			t.Fatalf("%s location got %d fixture %d", p.Name, o.StartLine, p.Start)
		}
		if _, ok := obsByID[o.ID()]; !ok {
			t.Fatal("observation id missing")
		}
		for _, d := range p.Depends {
			found := false
			for _, q := range r.Requirements {
				if q.From != o.ID() || q.Target != d.Target || q.Constraint != d.Constraint {
					continue
				}
				found = true
				if strings.HasPrefix(q.Target, "unresolved-") || strings.HasPrefix(q.Target, "[") {
					t.Fatalf("synthetic/native target %+v", q)
				}
				if q.Condition != d.Raw {
					t.Fatalf("%s dep raw got %q fixture %q", p.Name, q.Condition, d.Raw)
				}
				if want, uniq := byName[d.Target]; uniq {
					if len(q.Resolved) != 1 {
						t.Fatalf("%s dep %s unique name not resolved: %+v", p.Name, d.Target, q)
					}
					if obsByID[q.Resolved[0]].Component != want.Key.ID() {
						t.Fatalf("%s dep %s resolved to wrong identity", p.Name, d.Target)
					}
				} else if len(q.Resolved) != 0 {
					t.Fatalf("%s dep %s ambiguous name uniquely resolved by order: %+v", p.Name, d.Target, q)
				}
			}
			if !found {
				t.Fatalf("%s missing original gem dep %s (%s): %s", p.Name, d.Target, d.Raw, extra5149JSON(t, r.Requirements))
			}
		}
	}
	msf := comps["metasploit-framework@6.3.26"]
	if msf.Key.Source != "." {
		t.Fatalf("PATH remote source %q", msf.Key.Source)
	}
	from := byComp[msf.Key.ID()][0].ID()
	bom, sbomReqs, _ := parseSBOM(t, r)
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "metasploit-payloads" && q.Constraint == "= 2.0.148" {
			ok = true
			if len(q.Resolved) != 1 {
				t.Fatalf("payloads resolved %v", q.Resolved)
			}
			want := comps["metasploit-payloads@2.0.148"]
			if obsByID[q.Resolved[0]].Component != want.Key.ID() {
				t.Fatalf("SBOM resolved wrong payloads identity")
			}
		}
	}
	if !ok {
		t.Fatalf("SBOM lost original = 2.0.148: %s", extra5149JSON(t, sbomReqs))
	}
	_ = bom
}

func parsePnpmLock(raw []byte) (pkgs []origPkg, specs map[string]string) {
	specs = map[string]string{}
	section := ""
	var cur origPkg
	flush := func() {
		if cur.Name != "" || cur.Variant != "" {
			pkgs = append(pkgs, cur)
		}
		cur = origPkg{}
	}
	for _, ln := range strings.Split(string(raw), "\n") {
		trim := strings.TrimSpace(ln)
		indent := len(ln) - len(strings.TrimLeft(ln, " "))
		if trim == "specifiers:" {
			flush()
			section = "specifiers"
			continue
		}
		if trim == "packages:" {
			flush()
			section = "packages"
			continue
		}
		if trim == "dependencies:" || trim == "devDependencies:" {
			if section != "pkg" {
				section = trim
			}
			continue
		}
		if section == "specifiers" && indent == 2 && strings.Contains(trim, ":") {
			name, val, _ := strings.Cut(trim, ":")
			specs[strings.TrimSpace(name)] = strings.TrimSpace(val)
			continue
		}
		if section == "packages" && indent == 2 && strings.HasSuffix(trim, ":") {
			flush()
			key := strings.TrimSuffix(trim, ":")
			cur = origPkg{Variant: key}
			section = "pkg"
			continue
		}
		if section == "pkg" {
			switch {
			case strings.HasPrefix(trim, "resolution:"):
				if i := strings.Index(trim, "integrity: "); i >= 0 {
					rest := trim[i+len("integrity: "):]
					rest = strings.TrimSuffix(strings.TrimSpace(rest), "}")
					if j := strings.Index(rest, ","); j >= 0 {
						rest = rest[:j]
					}
					cur.Integrity = strings.TrimSpace(rest)
					cur.Verification = sriCanonical(cur.Integrity)
				}
				if i := strings.Index(trim, "tarball: "); i >= 0 {
					rest := trim[i+len("tarball: "):]
					rest = strings.TrimSuffix(strings.TrimSpace(rest), "}")
					if j := strings.Index(rest, ","); j >= 0 {
						rest = rest[:j]
					}
					cur.Source = strings.TrimSpace(rest)
				}
			case strings.HasPrefix(trim, "name:"):
				cur.Name = strings.TrimSpace(strings.TrimPrefix(trim, "name:"))
			case strings.HasPrefix(trim, "version:"):
				cur.Version = strings.TrimSpace(strings.TrimPrefix(trim, "version:"))
			case strings.HasPrefix(trim, "dev:"):
				// existing contract skips dev: true packages
				if strings.TrimSpace(strings.TrimPrefix(trim, "dev:")) == "true" {
					cur.Name = ""
					cur.Version = ""
				}
			}
		}
	}
	flush()
	for i := range pkgs {
		if pkgs[i].Name != "" {
			continue
		}
		key := strings.Trim(pkgs[i].Variant, "'\"")
		key = strings.TrimPrefix(key, "/")
		if strings.Contains(key, "@") && !strings.HasPrefix(key, "@") {
			n, v, _ := strings.Cut(key, "@")
			pkgs[i].Name, pkgs[i].Version = n, v
			continue
		}
		if slash := strings.LastIndex(key, "/"); slash > 0 {
			pkgs[i].Name, pkgs[i].Version = key[:slash], key[slash+1:]
		}
	}
	var keep []origPkg
	for _, p := range pkgs {
		if p.Name != "" && p.Version != "" {
			keep = append(keep, p)
		}
	}
	return keep, specs
}

func parsePoetryLock(raw []byte) []origPkg {
	var out []origPkg
	var cur origPkg
	section := ""
	line := 0
	flush := func() {
		if cur.Name != "" && cur.Version != "" {
			out = append(out, cur)
		}
		cur, section = origPkg{}, ""
	}
	for i, ln := range strings.Split(string(raw), "\n") {
		line = i + 1
		trim := strings.TrimSpace(ln)
		if trim == "[[package]]" {
			flush()
			cur.Start = line
			section = "pkg"
			continue
		}
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "[") && trim != "[[package]]" {
			section = trim
			continue
		}
		if section == "pkg" && cur.Start != 0 {
			cur.End = line
		}
		switch section {
		case "pkg":
			switch {
			case strings.HasPrefix(trim, "name = "):
				cur.Name = strings.Trim(strings.TrimPrefix(trim, "name = "), `"`)
			case strings.HasPrefix(trim, "version = "):
				cur.Version = strings.Trim(strings.TrimPrefix(trim, "version = "), `"`)
			case strings.HasPrefix(trim, "optional = "):
				cur.Dev = strings.TrimSpace(strings.TrimPrefix(trim, "optional = ")) == "true"
			case strings.HasPrefix(trim, "marker = "):
				cur.DeclaredPath = strings.Trim(strings.TrimPrefix(trim, "marker = "), `"`)
			case strings.HasPrefix(trim, "category = "):
				if strings.Trim(strings.TrimPrefix(trim, "category = "), `"`) == "dev" {
					cur.Name, cur.Version = "", ""
				}
			}
		case "[package.dependencies]":
			name, val, ok := strings.Cut(trim, " = ")
			if !ok {
				continue
			}
			d := origDep{Target: strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "_", "-"), ".", "-")), Raw: trim}
			val = strings.TrimSpace(val)
			if strings.HasPrefix(val, `"`) {
				d.Constraint = strings.Trim(val, `"`)
			} else if strings.HasPrefix(val, "{") {
				if i := strings.Index(val, `version = "`); i >= 0 {
					rest := val[i+len(`version = "`):]
					if j := strings.Index(rest, `"`); j >= 0 {
						d.Constraint = rest[:j]
					}
				}
			}
			cur.Depends = append(cur.Depends, d)
		case "[package.source]":
			if strings.HasPrefix(trim, "url = ") {
				cur.Source = strings.Trim(strings.TrimPrefix(trim, "url = "), `"`)
			}
			if strings.HasPrefix(trim, "resolved_reference = ") || strings.HasPrefix(trim, "reference = ") {
				ref := strings.Trim(strings.TrimPrefix(strings.TrimPrefix(trim, "resolved_reference = "), "reference = "), `"`)
				if cur.Source != "" && ref != "" {
					cur.Source += "#" + ref
				}
			}
		}
		if strings.HasPrefix(trim, "{file = ") && strings.Contains(trim, "hash = ") {
			if i := strings.Index(trim, `hash = "`); i >= 0 {
				h := trim[i+len(`hash = "`):]
				if j := strings.Index(h, `"`); j >= 0 {
					if cur.Integrity != "" {
						cur.Integrity += " "
					}
					cur.Integrity += h[:j]
				}
			}
		}
	}
	flush()
	for i := range out {
		var canon []string
		for _, tok := range strings.Fields(out[i].Integrity) {
			alg, hexpart, ok := strings.Cut(tok, ":")
			if ok && strings.ToLower(alg) == "sha256" && len(hexpart) == 64 {
				canon = append(canon, "sha256:"+hexpart)
			}
		}
		sort.Strings(canon)
		out[i].Verification = strings.Join(canon, " ")
	}
	return out
}

func testFullFieldPnpm(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/node_pnpm/pnpm-lock.yaml")
	if err != nil {
		t.Fatal(err)
	}
	orig, specs := parsePnpmLock(raw)
	old := loadOldScan(t, "pnpm")
	r := scanNamed(t, "pnpm", map[string]string{"pnpm-lock.yaml": "testdata/node_pnpm/pnpm-lock.yaml"})
	comps, obsByID, byComp := reportIndex(r)
	if len(orig) != len(r.Components) || len(old) != len(r.Components) {
		t.Fatalf("pnpm orig=%d old=%d new=%d", len(orig), len(old), len(r.Components))
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	for _, p := range orig {
		c, ok := comps[p.Name+"@"+p.Version]
		if !ok {
			t.Fatalf("missing %s@%s", p.Name, p.Version)
		}
		if p.Verification != "" && c.Key.Verification != p.Verification {
			t.Fatalf("%s verification got %q fixture %q", p.Name, c.Key.Verification, p.Verification)
		}
		if c.Key.Source != p.Source {
			t.Fatalf("%s source got %q fixture %q", p.Name, c.Key.Source, p.Source)
		}
		if c.Key.Architecture != "" || len(c.Licenses) != 0 {
			t.Fatalf("%s invented arch/license %+v", p.Name, c)
		}
		oc := oldBy[p.Name+"@"+p.Version]
		if oc.Verification != "" {
			t.Fatalf("old pnpm dump had verification for %s", p.Name)
		}
		if len(oc.FromFile) == 0 || oc.FromFile[0] != "pnpm-lock.yaml" || oc.FromAnalyzer[0] != "npmp-lang" {
			t.Fatalf("old dump lost from identity: %+v", oc)
		}
		hits := byComp[c.Key.ID()]
		if len(hits) == 0 {
			t.Fatalf("no observation for %s", p.Name)
		}
		o := hits[0]
		if o.Component != c.Key.ID() || o.File != "pnpm-lock.yaml" || o.Kind != "locked" {
			t.Fatalf("observation %+v", o)
		}
		if p.Integrity != "" && o.DeclaredIntegrity != p.Integrity {
			t.Fatalf("%s declaredIntegrity got %q fixture %q", p.Name, o.DeclaredIntegrity, p.Integrity)
		}
		if o.StartLine != 0 || o.EndLine != 0 {
			t.Fatalf("pnpm yaml map has no record spans, got %d-%d", o.StartLine, o.EndLine)
		}
		if _, ok := obsByID[o.ID()]; !ok {
			t.Fatal("observation id missing")
		}
		if spec := specs[p.Name]; spec != "" {
			found := false
			for _, q := range r.Requirements {
				if q.From != o.ID() || q.Target != p.Name {
					continue
				}
				found = true
				if q.Constraint != spec {
					t.Fatalf("%s specifier got %q fixture %q", p.Name, q.Constraint, spec)
				}
			}
			if !found {
				t.Fatalf("%s missing original specifier %s: %s", p.Name, spec, extra5149JSON(t, r.Requirements))
			}
		}
	}
	lodash := comps["lodash@4.17.21"]
	from := byComp[lodash.Key.ID()][0].ID()
	bom, sbomReqs, _ := parseSBOM(t, r)
	var lo *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "lodash" {
			lo = &bom.Components[i]
		}
	}
	if lo == nil || len(lo.Hashes) == 0 {
		t.Fatalf("SBOM lodash %+v", lo)
	}
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "lodash" && q.Constraint == "^4.17.21" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM lost original pnpm specifier: %s", extra5149JSON(t, sbomReqs))
	}
}

func testFullFieldPoetry(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/python_poetry/positive/poetry.lock")
	if err != nil {
		t.Fatal(err)
	}
	orig := parsePoetryLock(raw)
	old := loadOldScan(t, "poetry")
	r := scanNamed(t, "poetry", map[string]string{"poetry.lock": "testdata/python_poetry/positive/poetry.lock", "pyproject.toml": "testdata/python_poetry/positive/pyproject.toml"})
	comps, obsByID, byComp := reportIndex(r)
	if len(orig) != len(r.Components) || len(old) != len(r.Components) {
		t.Fatalf("poetry orig=%d old=%d new=%d", len(orig), len(old), len(r.Components))
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	for _, p := range orig {
		c, ok := comps[p.Name+"@"+p.Version]
		if !ok {
			t.Fatalf("missing %s@%s", p.Name, p.Version)
		}
		if p.Verification != "" && c.Key.Verification != p.Verification {
			t.Fatalf("%s verification got %q fixture %q", p.Name, c.Key.Verification, p.Verification)
		}
		if c.Key.Source != p.Source {
			t.Fatalf("%s source got %q fixture %q", p.Name, c.Key.Source, p.Source)
		}
		if c.Key.Architecture != "" || len(c.Licenses) != 0 {
			t.Fatalf("%s invented arch/license %+v", p.Name, c)
		}
		oc := oldBy[p.Name+"@"+p.Version]
		if oc.Verification != "" {
			t.Fatalf("old poetry dump had verification for %s", p.Name)
		}
		if len(oc.FromFile) == 0 || oc.FromFile[0] != "poetry.lock" || oc.FromAnalyzer[0] != "python-poetry-lang" {
			t.Fatalf("old dump lost from identity: %+v", oc)
		}
		for _, up := range oc.UpNames {
			if _, ok := comps[up]; !ok {
				t.Fatalf("old UpNames %q is not a locked identity", up)
			}
		}
		hits := byComp[c.Key.ID()]
		if len(hits) == 0 {
			t.Fatalf("no observation for %s", p.Name)
		}
		o := hits[0]
		if o.Component != c.Key.ID() || o.File != "poetry.lock" || o.Kind != "locked" {
			t.Fatalf("observation %+v", o)
		}
		if p.Dev && o.Scope != "optional" {
			t.Fatalf("%s optional scope lost: %+v", p.Name, o)
		}
		if p.Start != 0 && (o.StartLine != p.Start || o.EndLine != p.End) {
			t.Fatalf("%s location got %d-%d fixture %d-%d", p.Name, o.StartLine, o.EndLine, p.Start, p.End)
		}
		if _, ok := obsByID[o.ID()]; !ok {
			t.Fatal("observation id missing")
		}
		for _, d := range p.Depends {
			found := false
			for _, q := range r.Requirements {
				if q.From != o.ID() || q.Target != d.Target || q.Constraint != d.Constraint {
					continue
				}
				found = true
				if q.Constraint == c.Key.Version && d.Constraint != c.Key.Version {
					t.Fatalf("locked version used as original expression")
				}
			}
			if !found {
				t.Fatalf("%s missing original poetry dep %s %s: %s", p.Name, d.Target, d.Constraint, extra5149JSON(t, r.Requirements))
			}
		}
	}
	flask := comps["flask@1.1.4"]
	from := byComp[flask.Key.ID()][0].ID()
	bom, sbomReqs, _ := parseSBOM(t, r)
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "click" && q.Constraint == ">=5.1,<8.0" {
			ok = true
			want := comps["click@7.1.2"]
			if len(q.Resolved) != 1 || obsByID[q.Resolved[0]].Component != want.Key.ID() {
				t.Fatalf("SBOM click resolved wrong identity")
			}
		}
	}
	if !ok {
		t.Fatalf("SBOM lost original flask click range: %s", extra5149JSON(t, sbomReqs))
	}
	_ = bom
}

func parseGemspecOrig(raw []byte) origPkg {
	p := origPkg{}
	seen := map[string]bool{}
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		n := i + 1
		trim := strings.TrimSpace(line)
		if strings.Contains(trim, "Gem::Specification.new") && p.Start == 0 {
			p.Start = n
		}
		if p.Start != 0 {
			p.End = n
		}
		if v, ok := gemQuotedAssign(trim, ".name"); ok && p.Name == "" {
			p.Name = v
		}
		if v, ok := gemQuotedAssign(trim, ".version"); ok && p.Version == "" {
			p.Version = v
		}
		if v, ok := gemQuotedAssign(trim, ".homepage"); ok && p.Source == "" {
			p.Source = v
		}
		if strings.Contains(trim, ".licenses") && strings.Contains(trim, "[") {
			inner := trim[strings.Index(trim, "[")+1:]
			if j := strings.LastIndex(inner, "]"); j >= 0 {
				inner = inner[:j]
			}
			var lic []string
			for _, part := range strings.Split(inner, ",") {
				part = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), ".freeze"))
				part = strings.Trim(part, `'"`)
				if part != "" {
					lic = append(lic, part)
				}
			}
			p.LicenseRaw = strings.Join(lic, ", ")
			p.Provides = lic
		}
		method := ""
		switch {
		case strings.Contains(trim, "add_runtime_dependency"):
			method = "runtime"
		case strings.Contains(trim, "add_development_dependency"):
			method = "dev"
		case strings.Contains(trim, "add_dependency"):
			method = "generic"
		}
		if method == "" {
			continue
		}
		name := gemPercentQ(trim)
		if name == "" {
			continue
		}
		constraint := gemReqArray(trim)
		key := name + "\x00" + constraint
		if seen[key] {
			continue
		}
		seen[key] = true
		rawForm := name
		if constraint != "" {
			rawForm = name + "@" + constraint
		}
		p.Depends = append(p.Depends, origDep{Target: name, Constraint: constraint, Raw: rawForm})
		if method == "dev" {
			p.Patterns = append(p.Patterns, name)
		}
	}
	for p.End > p.Start && p.End <= len(lines) && strings.TrimSpace(lines[p.End-1]) == "" {
		p.End--
	}
	return p
}

func gemQuotedAssign(line, field string) (string, bool) {
	idx := strings.Index(line, field)
	if idx < 0 {
		return "", false
	}
	_, rhs, ok := strings.Cut(line[idx+len(field):], "=")
	if !ok {
		return "", false
	}
	rhs = strings.TrimSpace(rhs)
	rhs = strings.TrimSuffix(rhs, ".freeze")
	if len(rhs) < 2 || (rhs[0] != '"' && rhs[0] != '\'') {
		return "", false
	}
	quote := rhs[0]
	end := strings.IndexByte(rhs[1:], quote)
	if end < 0 {
		return "", false
	}
	return rhs[1 : 1+end], true
}

func gemPercentQ(line string) string {
	i := strings.Index(line, "%q<")
	if i < 0 {
		return ""
	}
	rest := line[i+3:]
	j := strings.IndexByte(rest, '>')
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func gemReqArray(line string) string {
	i := strings.Index(line, "[")
	j := strings.LastIndex(line, "]")
	if i < 0 || j <= i {
		return ""
	}
	var parts []string
	for _, part := range strings.Split(line[i+1:j], ",") {
		part = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), ".freeze"))
		part = strings.Trim(part, `'"`)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}

func parsePackagingOrig(raw []byte) origPkg {
	p := origPkg{Start: 1}
	for i, line := range strings.Split(string(raw), "\n") {
		n := i + 1
		if strings.TrimRight(line, "\r") == "" {
			if p.Name != "" {
				p.End = n - 1
				break
			}
			continue
		}
		p.End = n
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.ToLower(key) {
		case "name":
			p.Name = val
		case "version":
			p.Version = val
		case "license":
			p.LicenseRaw = val
		case "home-page":
			p.Source = val
		}
	}
	if p.End < 1 {
		p.End = 1
	}
	return p
}

func testFullFieldGemspec(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/ruby_gemspec/positive/multiple_licenses.gemspec")
	if err != nil {
		t.Fatal(err)
	}
	orig := parseGemspecOrig(raw)
	old := loadOldScan(t, "gemspec")
	r := scanNamed(t, "gemspec", map[string]string{"gems/specifications/test-unit.gemspec": "testdata/ruby_gemspec/positive/multiple_licenses.gemspec"})
	comps, obsByID, byComp := reportIndex(r)
	if orig.Name != "test-unit" || orig.Version != "3.3.7" {
		t.Fatalf("independent fixture parse %+v", orig)
	}
	if len(r.Components) != 1 || len(old) != 1 {
		t.Fatalf("gemspec orig identity 1 old=%d new=%d", len(old), len(r.Components))
	}
	c, ok := comps["test-unit@3.3.7"]
	if !ok {
		t.Fatal("missing test-unit@3.3.7")
	}
	if c.Key.Source != orig.Source || orig.Source != "http://test-unit.github.io/" {
		t.Fatalf("homepage source got %q fixture %q", c.Key.Source, orig.Source)
	}
	if c.Key.Architecture != "" || c.Key.Verification != "" {
		t.Fatalf("gemspec has no CPU arch or checksum, got %+v", c.Key)
	}
	if c.Key.Ecosystem != "gem" {
		t.Fatalf("ecosystem %q", c.Key.Ecosystem)
	}
	wantLicenses := append([]string(nil), orig.Provides...)
	sort.Strings(wantLicenses)
	if !reflect.DeepEqual(c.Licenses, wantLicenses) {
		t.Fatalf("license boundaries got %q want %q", c.Licenses, wantLicenses)
	}
	oc := old[0]
	if oc.Name != orig.Name || oc.Version != orig.Version {
		t.Fatalf("old dump identity %+v", oc)
	}
	if oc.Verification != "" {
		t.Fatal("old gemspec dump had verification")
	}
	if strings.Join(oc.License, ", ") != orig.LicenseRaw {
		t.Fatalf("old dump licenses %v fixture %q", oc.License, orig.LicenseRaw)
	}
	if oc.DependsAnd != nil || oc.UpNames != nil {
		t.Fatalf("old dump unexpectedly had deps: %+v", oc)
	}
	if len(oc.FromFile) == 0 || oc.FromFile[0] != "gems/specifications/test-unit.gemspec" || oc.FromAnalyzer[0] != "ruby-gemspec-lang" {
		t.Fatalf("old dump lost from identity: %+v", oc)
	}
	hits := byComp[c.Key.ID()]
	if len(hits) == 0 {
		t.Fatal("no observation")
	}
	o := hits[0]
	if o.Component != c.Key.ID() || o.File != "gems/specifications/test-unit.gemspec" || o.Kind != "locked" {
		t.Fatalf("observation %+v", o)
	}
	if orig.Start != 0 && (o.StartLine != orig.Start || o.EndLine != orig.End) {
		t.Fatalf("location got %d-%d fixture %d-%d", o.StartLine, o.EndLine, orig.Start, orig.End)
	}
	if _, ok := obsByID[o.ID()]; !ok {
		t.Fatal("observation id missing")
	}
	dev := map[string]bool{}
	for _, name := range orig.Patterns {
		dev[name] = true
	}
	if len(orig.Depends) != 6 {
		t.Fatalf("independent add_* unique count %d: %+v", len(orig.Depends), orig.Depends)
	}
	for _, d := range orig.Depends {
		found := false
		for _, q := range r.Requirements {
			if q.From != o.ID() || q.Target != d.Target || q.Constraint != d.Constraint {
				continue
			}
			found = true
			if q.Condition != d.Raw {
				t.Fatalf("%s condition got %q fixture %q", d.Target, q.Condition, d.Raw)
			}
			wantScope := ""
			if dev[d.Target] {
				wantScope = "dev"
			}
			if q.Scope != wantScope {
				t.Fatalf("%s scope got %q want %q", d.Target, q.Scope, wantScope)
			}
			if len(q.Resolved) != 0 {
				t.Fatalf("gemspec range uniquely resolved: %+v", q)
			}
			if strings.HasPrefix(q.Target, "unresolved-") {
				t.Fatalf("synthetic %q", q.Target)
			}
		}
		if !found {
			t.Fatalf("missing original add_* %s %s: %s", d.Target, d.Constraint, extra5149JSON(t, r.Requirements))
		}
	}
	from := o.ID()
	bom, sbomReqs, _ := parseSBOM(t, r)
	var tu *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "test-unit" {
			tu = &bom.Components[i]
		}
	}
	if tu == nil {
		t.Fatal("SBOM missing test-unit")
	}
	srcOK := false
	for _, p := range tu.Properties {
		if p.Name == "sca:source" && p.Value == orig.Source {
			srcOK = true
		}
	}
	if !srcOK {
		t.Fatalf("SBOM lost homepage source: %+v", tu.Properties)
	}
	var bomLicenses []string
	for _, choice := range tu.Licenses {
		if choice.License == nil || choice.Expression != "" {
			t.Fatalf("unexpected license choice %+v", choice)
		}
		bomLicenses = append(bomLicenses, choice.License.Name)
	}
	sort.Strings(bomLicenses)
	if !reflect.DeepEqual(bomLicenses, wantLicenses) {
		t.Fatalf("SBOM license boundaries got %q want %q", bomLicenses, wantLicenses)
	}
	okReq := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "power_assert" && q.Constraint == ">= 0" && q.Scope == "" {
			okReq = true
		}
	}
	if !okReq {
		t.Fatalf("SBOM lost original runtime add_*: %s", extra5149JSON(t, sbomReqs))
	}
	okReq = false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "bundler" && q.Constraint == ">= 0" && q.Scope == "dev" {
			okReq = true
		}
	}
	if !okReq {
		t.Fatalf("SBOM lost original development add_*: %s", extra5149JSON(t, sbomReqs))
	}
}

func testFullFieldPackaging(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/python_packaging/dist-info/METADATA")
	if err != nil {
		t.Fatal(err)
	}
	orig := parsePackagingOrig(raw)
	old := loadOldScan(t, "packaging")
	r := scanNamed(t, "packaging", map[string]string{"x.dist-info/METADATA": "testdata/python_packaging/dist-info/METADATA"})
	comps, obsByID, byComp := reportIndex(r)
	if orig.Name != "distlib" || orig.Version != "0.3.1" || orig.Source != "https://bitbucket.org/pypa/distlib" {
		t.Fatalf("independent METADATA parse %+v", orig)
	}
	if len(r.Components) != 1 || len(old) != 1 {
		t.Fatalf("packaging orig identity 1 old=%d new=%d", len(old), len(r.Components))
	}
	c, ok := comps["distlib@0.3.1"]
	if !ok {
		t.Fatal("missing distlib@0.3.1")
	}
	if c.Key.Source != orig.Source {
		t.Fatalf("home-page source got %q fixture %q", c.Key.Source, orig.Source)
	}
	if c.Key.Architecture != "" || c.Key.Verification != "" {
		t.Fatalf("METADATA has no CPU arch or payload hash, got %+v", c.Key)
	}
	if c.Key.Ecosystem != "pypi" {
		t.Fatalf("ecosystem %q", c.Key.Ecosystem)
	}
	if !strings.Contains(strings.Join(c.Licenses, " "), orig.LicenseRaw) {
		t.Fatalf("license got %v fixture %q", c.Licenses, orig.LicenseRaw)
	}
	oc := old[0]
	if oc.Name != orig.Name || oc.Version != orig.Version {
		t.Fatalf("old dump identity %+v", oc)
	}
	if oc.Verification != "" {
		t.Fatal("old packaging dump had verification")
	}
	if len(oc.License) == 0 || oc.License[0] != orig.LicenseRaw {
		t.Fatalf("old dump license %v fixture %q", oc.License, orig.LicenseRaw)
	}
	if oc.DependsAnd != nil || oc.UpNames != nil {
		t.Fatalf("old dump unexpectedly had deps: %+v", oc)
	}
	if len(oc.FromFile) == 0 || oc.FromFile[0] != "x.dist-info/METADATA" || oc.FromAnalyzer[0] != "python-packaging-lang" {
		t.Fatalf("old dump lost from identity: %+v", oc)
	}
	hits := byComp[c.Key.ID()]
	if len(hits) == 0 {
		t.Fatal("no observation")
	}
	o := hits[0]
	if o.Component != c.Key.ID() || o.File != "x.dist-info/METADATA" || o.Kind != "locked" {
		t.Fatalf("observation %+v", o)
	}
	if orig.Start != 0 && (o.StartLine != orig.Start || o.EndLine != orig.End) {
		t.Fatalf("location got %d-%d fixture %d-%d", o.StartLine, o.EndLine, orig.Start, orig.End)
	}
	if _, ok := obsByID[o.ID()]; !ok {
		t.Fatal("observation id missing")
	}
	if len(r.Requirements) != 0 {
		t.Fatalf("distlib METADATA has no Requires-Dist, got %s", extra5149JSON(t, r.Requirements))
	}
	bom, _, sbomObs := parseSBOM(t, r)
	var dl *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "distlib" && bom.Components[i].Version == "0.3.1" {
			dl = &bom.Components[i]
		}
	}
	if dl == nil {
		t.Fatal("SBOM missing distlib")
	}
	srcOK := false
	for _, p := range dl.Properties {
		if p.Name == "sca:source" && p.Value == orig.Source {
			srcOK = true
		}
	}
	if !srcOK {
		t.Fatalf("SBOM lost Home-page: %+v", dl.Properties)
	}
	if len(sbomObs) != len(r.Observations) {
		t.Fatalf("SBOM observations %d report %d", len(sbomObs), len(r.Observations))
	}
}

func scanReportFiles(t *testing.T, id string, files map[string]string) *model.Report {
	t.Helper()
	in := fstest.MapFS{}
	for path, file := range files {
		raw, err := fixtures.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		in[path] = &fstest.MapFile{Data: raw}
	}
	r, err := ScanReport(context.Background(), in, WithSnapshotID("full-field-"+id))
	if r == nil {
		t.Fatalf("nil report err=%v", err)
	}
	return r
}

func pomRepoFiles(t *testing.T, files map[string]string) map[string]string {
	t.Helper()
	root := "analyzer/dep-parser/java/pom/testdata/repository"
	err := fixtures.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files["repository/"+filepath.ToSlash(rel)] = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func requireEdge(t *testing.T, r *model.Report, from, target, constraint, scope, condition string) model.Requirement {
	t.Helper()
	for _, q := range r.Requirements {
		if q.From != from || q.Target != target || q.Constraint != constraint {
			continue
		}
		if q.Scope != scope || q.Condition != condition {
			t.Fatalf("%s -> %s %s scope/condition got %q/%q want %q/%q", from, target, constraint, q.Scope, q.Condition, scope, condition)
		}
		return q
	}
	t.Fatalf("missing original edge %s -> %s %q scope=%q: %s", from, target, constraint, scope, extra5149JSON(t, r.Requirements))
	return model.Requirement{}
}

func testFullFieldPOM(t *testing.T) {
	t.Run("matrix-old", func(t *testing.T) {
		raw, err := fixtures.ReadFile("testdata/java_pom/positive/pom.xml")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "<name>Apache 2.0</name>") || !strings.Contains(string(raw), `xmlns="http://maven.apache.org/POM/4.0.0"`) {
			t.Fatal("matrix pom lost original license token or default Maven xmlns")
		}
		old := loadOldScan(t, "pom")
		r := scanNamed(t, "pom", map[string]string{"pom.xml": "testdata/java_pom/positive/pom.xml"})
		comps, obsByID, byComp := reportIndex(r)
		if len(r.Components) != 1 || len(old) != 1 {
			t.Fatalf("pom orig identity 1 old=%d new=%d", len(old), len(r.Components))
		}
		c, ok := comps["com.example:example@1.0.0"]
		if !ok {
			t.Fatal("missing com.example:example@1.0.0")
		}
		if c.Key.Ecosystem != "maven" {
			t.Fatalf("ecosystem %q", c.Key.Ecosystem)
		}
		if c.Key.Source != "" || c.Key.Architecture != "" || c.Key.Verification != "" {
			t.Fatalf("POM project has no source/arch/digest: %+v", c.Key)
		}
		if strings.Join(c.Licenses, ", ") != "Apache 2.0" {
			t.Fatalf("original license token got %v", c.Licenses)
		}
		oc := old[0]
		if oc.Name != "com.example:example" || oc.Version != "1.0.0" {
			t.Fatalf("old dump identity %+v", oc)
		}
		if oc.Verification != "" {
			t.Fatal("old pom dump had verification")
		}
		if len(oc.License) != 1 || oc.License[0] != "Apache-2.0" {
			t.Fatalf("old dump normalized license %v", oc.License)
		}
		if oc.DependsAnd != nil || oc.UpNames != nil {
			t.Fatalf("old dump unexpectedly had deps: %+v", oc)
		}
		if len(oc.FromFile) == 0 || oc.FromFile[0] != "pom.xml" || oc.FromAnalyzer[0] != "pom-lang" {
			t.Fatalf("old dump lost from identity: %+v", oc)
		}
		hits := byComp[c.Key.ID()]
		if len(hits) == 0 {
			t.Fatal("no observation")
		}
		o := hits[0]
		if o.Component != c.Key.ID() || o.File != "pom.xml" || o.Kind != "declared" {
			t.Fatalf("observation %+v", o)
		}
		if _, ok := obsByID[o.ID()]; !ok {
			t.Fatal("observation id missing")
		}
		bom, _, sbomObs := parseSBOM(t, r)
		var ex *dxtypes.BOMComponent
		for i := range bom.Components {
			if bom.Components[i].Name == "com.example:example" && bom.Components[i].Version == "1.0.0" {
				ex = &bom.Components[i]
			}
		}
		if ex == nil {
			t.Fatal("SBOM missing example")
		}
		licJSON, _ := json.Marshal(ex.Licenses)
		if !strings.Contains(string(licJSON), "Apache 2.0") {
			t.Fatalf("SBOM lost original license token: %s", licJSON)
		}
		if len(sbomObs) != len(r.Observations) {
			t.Fatalf("SBOM observations %d report %d", len(sbomObs), len(r.Observations))
		}
	})

	t.Run("range", func(t *testing.T) {
		r := scanReportFiles(t, "pom-range", map[string]string{"pom.xml": "testdata/java_pom/requirements/pom.xml"})
		if r.Complete {
			t.Fatal("range POM must not claim complete while the ranged coordinate is absent from the snapshot")
		}
		comps, _, byComp := reportIndex(r)
		root, ok := comps["com.example:example@2.0.0"]
		if !ok {
			t.Fatal("missing root")
		}
		if strings.Join(root.Licenses, ", ") != "Apache 2.0" {
			t.Fatalf("license %v", root.Licenses)
		}
		api, ok := comps["org.example:example-api@"]
		if !ok {
			t.Fatalf("range must not be promoted to a concrete version: %s", extra5149JSON(t, r.Components))
		}
		if api.Key.Version != "" {
			t.Fatalf("range promoted: %q", api.Key.Version)
		}
		from := byComp[root.Key.ID()][0].ID()
		q := requireEdge(t, r, from, "org.example:example-api", "(,1.0]", "", "")
		if len(q.Resolved) != 0 {
			t.Fatalf("uncertain version uniquely resolved: %+v", q)
		}
		if q.Scope == "runtime" {
			t.Fatalf("invented runtime: %+v", q)
		}
	})

	t.Run("properties-provided", func(t *testing.T) {
		files := pomRepoFiles(t, map[string]string{"pom.xml": "analyzer/dep-parser/java/pom/testdata/happy/pom.xml"})
		r := scanReportFiles(t, "pom-happy", files)
		comps, obsByID, byComp := reportIndex(r)
		root, ok := comps["com.example:happy@1.0.0"]
		if !ok {
			t.Fatal("missing happy")
		}
		if strings.Join(root.Licenses, ", ") != "BSD-3-Clause" {
			t.Fatalf("license %v", root.Licenses)
		}
		api, ok := comps["org.example:example-api@1.7.30"]
		if !ok {
			t.Fatal("property interpolation did not produce example-api@1.7.30")
		}
		prov, ok := comps["org.example:example-provided@999"]
		if !ok {
			t.Fatal("provided dependency dropped")
		}
		from := byComp[root.Key.ID()][0].ID()
		q := requireEdge(t, r, from, "org.example:example-api", "${api.version}", "", "")
		if len(q.Resolved) != 1 || obsByID[q.Resolved[0]].Component != api.Key.ID() {
			t.Fatalf("in-POM property must resolve uniquely: %+v", q)
		}
		q = requireEdge(t, r, from, "org.example:example-provided", "999", "provided", "")
		if len(q.Resolved) != 1 || obsByID[q.Resolved[0]].Component != prov.Key.ID() {
			t.Fatalf("provided resolved %+v", q)
		}
		incomplete := false
		for _, d := range r.Diagnostics {
			if d.Code == "evidence_insufficient" && strings.Contains(d.Reason, "example-provided:999") {
				incomplete = true
			}
		}
		if r.Complete || !incomplete {
			t.Fatalf("missing provided POM metadata must stay incomplete, not invent host lookup: complete=%v diag=%s", r.Complete, extra5149JSON(t, r.Diagnostics))
		}
		bom, sbomReqs, _ := parseSBOM(t, r)
		okReq := false
		for _, q := range sbomReqs {
			if q.From == from && q.Target == "org.example:example-api" && q.Constraint == "${api.version}" {
				okReq = true
			}
		}
		if !okReq {
			t.Fatalf("SBOM lost original property constraint: %s", extra5149JSON(t, sbomReqs))
		}
		var happy *dxtypes.BOMComponent
		for i := range bom.Components {
			if bom.Components[i].Name == "com.example:happy" {
				happy = &bom.Components[i]
			}
		}
		if happy == nil {
			t.Fatal("SBOM missing happy")
		}
	})

	t.Run("bom-import", func(t *testing.T) {
		files := pomRepoFiles(t, map[string]string{"pom.xml": "analyzer/dep-parser/java/pom/testdata/import-dependency-management/pom.xml"})
		r := scanReportFiles(t, "pom-bom", files)
		if !r.Complete {
			t.Fatalf("snapshot BOM must complete: %s", extra5149JSON(t, r.Diagnostics))
		}
		comps, obsByID, byComp := reportIndex(r)
		root := comps["com.example:import@2.0.0"]
		api := comps["org.example:example-api@1.7.30"]
		if root.Key.Name == "" || api.Key.Name == "" {
			t.Fatalf("BOM-managed identity missing: %s", extra5149JSON(t, r.Components))
		}
		if _, ok := comps["org.example:example-dependency-management@2.2.2"]; ok {
			t.Fatal("BOM import must not become an installed component")
		}
		from := byComp[root.Key.ID()][0].ID()
		q := requireEdge(t, r, from, "org.example:example-api", "", "", "")
		if len(q.Resolved) != 1 || obsByID[q.Resolved[0]].Component != api.Key.ID() {
			t.Fatalf("BOM-managed empty declaration must resolve from snapshot BOM: %+v", q)
		}
	})

	t.Run("exclusions", func(t *testing.T) {
		files := pomRepoFiles(t, map[string]string{"pom.xml": "analyzer/dep-parser/java/pom/testdata/exclusions/pom.xml"})
		r := scanReportFiles(t, "pom-excl", files)
		if !r.Complete {
			t.Fatalf("snapshot exclusions: %s", extra5149JSON(t, r.Diagnostics))
		}
		comps, obsByID, byComp := reportIndex(r)
		if _, ok := comps["org.example:example-api@2.0.0"]; ok {
			t.Fatal("excluded transitive example-api must not appear")
		}
		if _, ok := comps["org.example:example-api@"]; ok {
			t.Fatal("excluded example-api appeared without version")
		}
		root := comps["com.example:exclusions@3.0.0"]
		nested := comps["org.example:example-nested@3.3.3"]
		dep := comps["org.example:example-dependency@1.2.3"]
		if root.Key.Name == "" || nested.Key.Name == "" || dep.Key.Name == "" {
			t.Fatalf("exclusion kept identities missing: %s", extra5149JSON(t, r.Components))
		}
		from := byComp[root.Key.ID()][0].ID()
		q := requireEdge(t, r, from, "org.example:example-nested", "3.3.3", "", "")
		if len(q.Resolved) != 1 || obsByID[q.Resolved[0]].Component != nested.Key.ID() {
			t.Fatalf("direct nested %+v", q)
		}
		for _, q := range r.Requirements {
			if q.From == from && strings.Contains(q.Target, "example-api") {
				t.Fatalf("exclusion must not keep example-api as a requirement target: %+v", q)
			}
		}
	})

	t.Run("parent-relative", func(t *testing.T) {
		files := pomRepoFiles(t, map[string]string{
			"pom.xml":        "analyzer/dep-parser/java/pom/testdata/parent-relative-path/pom.xml",
			"parent/pom.xml": "analyzer/dep-parser/java/pom/testdata/parent-relative-path/parent/pom.xml",
		})
		r := scanReportFiles(t, "pom-parent", files)
		if !r.Complete {
			t.Fatalf("relative parent in snapshot: %s", extra5149JSON(t, r.Diagnostics))
		}
		comps, obsByID, byComp := reportIndex(r)
		child := comps["com.example:child@1.0.0"]
		parent := comps["com.example:parent@1.0.0"]
		api := comps["org.example:example-api@1.7.30"]
		if child.Key.Name == "" || parent.Key.Name == "" || api.Key.Name == "" {
			t.Fatalf("parent/child identities: %s", extra5149JSON(t, r.Components))
		}
		if strings.Join(child.Licenses, ", ") != "Apache 2.0" {
			t.Fatalf("child license %v", child.Licenses)
		}
		from := byComp[child.Key.ID()][0].ID()
		q := requireEdge(t, r, from, "org.example:example-api", "${api.version}", "", "")
		if len(q.Resolved) != 1 || obsByID[q.Resolved[0]].Component != api.Key.ID() {
			t.Fatalf("parent property interpolation: %+v", q)
		}
	})

	t.Run("missing-parent", func(t *testing.T) {
		r := scanReportFiles(t, "pom-positive2", map[string]string{"pom.xml": "testdata/java_pom/positive2/pom.xml"})
		if r.Complete {
			t.Fatal("missing Spring parent must not complete via host cache or network")
		}
		parentMissing := false
		for _, d := range r.Diagnostics {
			if d.Code == "evidence_insufficient" && strings.Contains(d.Reason, "spring-boot-starter-parent:2.6.3") {
				parentMissing = true
			}
		}
		if !parentMissing {
			t.Fatalf("missing parent diagnostic: %s", extra5149JSON(t, r.Diagnostics))
		}
		comps, _, byComp := reportIndex(r)
		root := comps["com.example:demo@0.0.1-SNAPSHOT"]
		if root.Key.Name == "" {
			t.Fatal("demo identity dropped")
		}
		from := byComp[root.Key.ID()][0].ID()
		requireEdge(t, r, from, "org.springframework.boot:spring-boot-starter-web", "", "", "")
		requireEdge(t, r, from, "org.springframework.boot:spring-boot-starter-test", "", "test", "")
		requireEdge(t, r, from, "mysql:mysql-connector-java", "", "runtime", "")
		q := requireEdge(t, r, from, "org.mybatis.spring.boot:mybatis-spring-boot-starter", "2.2.0", "", "")
		if len(q.Resolved) != 1 {
			t.Fatalf("versioned snapshot-local declaration should resolve the declared component: %+v", q)
		}
		web := comps["org.springframework.boot:spring-boot-starter-web@"]
		if web.Key.Version != "" {
			t.Fatalf("unversioned parent-managed dep invented a version: %q", web.Key.Version)
		}
	})
}

func testFullFieldGradle(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/java_gradle/positive.lockfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "com.example:example:0.0.1=classpath") {
		t.Fatalf("frozen gradle lock syntax drifted: %q", raw)
	}
	old := loadOldScan(t, "gradle")
	r := scanNamed(t, "gradle", map[string]string{"gradle.lockfile": "testdata/java_gradle/positive.lockfile"})
	comps, obsByID, byComp := reportIndex(r)
	if len(r.Components) != 1 || len(old) != 1 {
		t.Fatalf("gradle orig identity 1 old=%d new=%d", len(old), len(r.Components))
	}
	c, ok := comps["com.example:example@0.0.1"]
	if !ok {
		t.Fatal("missing com.example:example@0.0.1")
	}
	if c.Key.Ecosystem != "maven" {
		t.Fatalf("ecosystem %q", c.Key.Ecosystem)
	}
	if c.Key.Source != "" || c.Key.Architecture != "" || c.Key.Verification != "" || len(c.Licenses) != 0 {
		t.Fatalf("lockfile has no source/arch/digest/license: %+v", c)
	}
	oc := old[0]
	if oc.Name != "com.example:example" || oc.Version != "0.0.1" {
		t.Fatalf("old dump identity %+v", oc)
	}
	if oc.Verification != "" || oc.License != nil || oc.DependsAnd != nil {
		t.Fatalf("old dump extra fields %+v", oc)
	}
	if len(oc.FromFile) == 0 || oc.FromFile[0] != "gradle.lockfile" || oc.FromAnalyzer[0] != "gradle-lang" {
		t.Fatalf("old dump lost from identity: %+v", oc)
	}
	hits := byComp[c.Key.ID()]
	if len(hits) != 1 {
		t.Fatalf("observations %+v", hits)
	}
	o := hits[0]
	if o.Component != c.Key.ID() || o.File != "gradle.lockfile" || o.Kind != "locked" {
		t.Fatalf("observation %+v", o)
	}
	if o.Scope != "classpath" {
		t.Fatalf("lock scope got %q", o.Scope)
	}
	if o.StartLine != 4 || o.EndLine != 4 {
		t.Fatalf("location %d-%d", o.StartLine, o.EndLine)
	}
	if _, ok := obsByID[o.ID()]; !ok {
		t.Fatal("observation id missing")
	}
	if len(r.Requirements) != 0 {
		t.Fatalf("gradle lockfile has no declared edges: %s", extra5149JSON(t, r.Requirements))
	}
	bom, sbomReqs, sbomObs := parseSBOM(t, r)
	var ex *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "com.example:example" && bom.Components[i].Version == "0.0.1" {
			ex = &bom.Components[i]
		}
	}
	if ex == nil {
		t.Fatal("SBOM missing example")
	}
	if len(ex.Hashes) != 0 {
		t.Fatalf("lockfile has no digest: %+v", ex.Hashes)
	}
	if len(sbomReqs) != 0 {
		t.Fatalf("SBOM invented gradle edges: %s", extra5149JSON(t, sbomReqs))
	}
	if len(sbomObs) != len(r.Observations) {
		t.Fatalf("SBOM observations %d report %d", len(sbomObs), len(r.Observations))
	}

	happy := scanNamed(t, "gradle-happy", map[string]string{"gradle.lockfile": "analyzer/dep-parser/java/gradle/testdata/happy.lockfile"})
	hcomps, _, happyBy := reportIndex(happy)
	want := map[string]string{
		"cglib:cglib-nodep@2.1.2":                        "testRuntimeClasspath,classpath",
		"org.springframework:spring-asm@3.1.3.RELEASE":   "classpath",
		"org.springframework:spring-beans@5.0.5.RELEASE": "compileClasspath, runtimeClasspath",
	}
	for key, scope := range want {
		c, ok := hcomps[key]
		if !ok {
			t.Fatalf("missing %s", key)
		}
		hits := happyBy[c.Key.ID()]
		if len(hits) == 0 || hits[0].Scope != scope {
			t.Fatalf("%s scope got %+v want %q", key, hits, scope)
		}
		if hits[0].Kind != "locked" || hits[0].File != "gradle.lockfile" {
			t.Fatalf("%s observation %+v", key, hits[0])
		}
	}
	if len(happy.Requirements) != 0 {
		t.Fatalf("happy lockfile invented edges: %s", extra5149JSON(t, happy.Requirements))
	}
	cglib := hcomps["cglib:cglib-nodep@2.1.2"]
	if happyBy[cglib.Key.ID()][0].StartLine != 4 {
		t.Fatalf("comment lines must not shift lock locations: %+v", happyBy[cglib.Key.ID()])
	}
}

func testFullFieldJAR(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/java_jar/positive/test.jar")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	hasPomProps, hasManifest := false, false
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if base == "pom.properties" {
			hasPomProps = true
		}
		if base == "MANIFEST.MF" {
			hasManifest = true
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			man, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(man), "Bundle-SymbolicName: org.apache.tomcat-embed-websocket") || !strings.Contains(string(man), "Bundle-Version: 9.0.65") {
				t.Fatalf("fixture MANIFEST drifted: %s", man[:200])
			}
		}
	}
	if hasPomProps {
		t.Fatal("matrix test.jar is manifest-only; pom.properties would be a different material")
	}
	if !hasManifest {
		t.Fatal("missing MANIFEST.MF")
	}
	old := loadOldScan(t, "jar")
	r := scanNamed(t, "jar", map[string]string{"x.jar": "testdata/java_jar/positive/test.jar"})
	comps, obsByID, byComp := reportIndex(r)
	if len(r.Components) != 1 || len(old) != 1 {
		t.Fatalf("jar orig 1 old=%d new=%d", len(old), len(r.Components))
	}
	c, ok := comps["org.apache:tomcat-embed-websocket@9.0.65"]
	if !ok {
		t.Fatal("missing org.apache:tomcat-embed-websocket@9.0.65")
	}
	if c.Key.Ecosystem != "maven" {
		t.Fatalf("ecosystem %q", c.Key.Ecosystem)
	}
	if c.Key.Verification != "" || c.Key.Architecture != "" || c.Key.Source != "" || len(c.Licenses) != 0 {
		t.Fatalf("manifest/pom.properties have no digest/arch/source/license: %+v", c)
	}
	oc := old[0]
	if oc.Name != c.Key.Name || oc.Version != c.Key.Version {
		t.Fatalf("old dump identity %+v", oc)
	}
	if oc.Verification != "" || oc.License != nil {
		t.Fatalf("old dump invented digest/license %+v", oc)
	}
	if oc.FromFile[0] != "x.jar" || oc.FromAnalyzer[0] != "jar-lang" {
		t.Fatalf("old dump from %+v", oc)
	}
	hits := byComp[c.Key.ID()]
	if len(hits) != 1 {
		t.Fatalf("observations %+v", hits)
	}
	o := hits[0]
	if o.Component != c.Key.ID() || o.Kind != "inferred" {
		t.Fatalf("manifest fallback must stay inferred: %+v", o)
	}
	if o.File != "x.jar!/META-INF/MANIFEST.MF" {
		t.Fatalf("manifest path %q", o.File)
	}
	if o.StartLine != 0 || o.EndLine != 0 {
		t.Fatalf("jar entries have no source lines: %+v", o)
	}
	if _, ok := obsByID[o.ID()]; !ok {
		t.Fatal("observation id missing")
	}
	if len(r.Requirements) != 0 {
		t.Fatalf("this jar has no declared edges: %s", extra5149JSON(t, r.Requirements))
	}
	bom, sbomReqs, sbomObs := parseSBOM(t, r)
	var tw *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "org.apache:tomcat-embed-websocket" {
			tw = &bom.Components[i]
		}
	}
	if tw == nil {
		t.Fatal("SBOM missing tomcat-embed-websocket")
	}
	if len(tw.Hashes) != 0 {
		t.Fatalf("SBOM invented jar digest: %+v", tw.Hashes)
	}
	if len(sbomReqs) != 0 || len(sbomObs) != len(r.Observations) {
		t.Fatalf("SBOM reqs=%d obs=%d", len(sbomReqs), len(sbomObs))
	}

	nested := scanNamed(t, "jar-nested", map[string]string{"nested.jar": "analyzer/testdata/upstream_jar/nested.jar"})
	ncomps, _, nby := reportIndex(nested)
	want := map[string]string{
		"test:nested@0.0.1":  "nested.jar!/META-INF/nested/pom.properties",
		"test:nested2@0.0.2": "nested.jar!/META-INF/jars/nested2.jar!/META-INF/nested2/pom.properties",
		"test:nested3@0.0.3": "nested.jar!/META-INF/jars/nested2.jar!/META-INF/jars/nested3.jar!/META-INF/nested3/pom.properties",
	}
	if len(ncomps) != 3 {
		t.Fatalf("nested identities %d: %s", len(ncomps), extra5149JSON(t, nested.Components))
	}
	for key, file := range want {
		c, ok := ncomps[key]
		if !ok {
			t.Fatalf("missing %s", key)
		}
		hits := nby[c.Key.ID()]
		if len(hits) == 0 || hits[0].File != file || hits[0].Kind != "locked" {
			t.Fatalf("%s observation %+v want file %s", key, hits, file)
		}
	}
}

func testFullFieldGoBinary(t *testing.T) {
	old := loadOldScan(t, "gobinary")
	r := scanNamed(t, "gobinary", map[string]string{"app": "testdata/go_binary/go-binary"})
	comps, obsByID, byComp := reportIndex(r)
	if len(r.Components) != 3 || len(old) != 3 {
		t.Fatalf("gobinary orig=3 old=%d new=%d", len(old), len(r.Components))
	}
	want := map[string]string{
		"github.com/aquasecurity/go-pep440-version@v0.0.0-20210121094942-22b2f8951d46": "h1:vmXNl+HDfqqXgr0uY1UgK1GAhps8nbAAtqHNBcgyf+4=",
		"github.com/aquasecurity/go-version@v0.0.0-20210121072130-637058cfe492":        "h1:rcEG5HI490FF0a7zuvxOxen52ddygCfNVjP0XOCMl+M=",
		"golang.org/x/xerrors@v0.0.0-20200804184101-5ec99f83aff1":                      "h1:go1bK/D/BFZV2I8cIQd1NKEZ+0owSTG1fDTci4IqFcE=",
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	for key, sum := range want {
		c, ok := comps[key]
		if !ok {
			t.Fatalf("missing %s", key)
		}
		if c.Key.Ecosystem != "golang" {
			t.Fatalf("%s ecosystem %q", key, c.Key.Ecosystem)
		}
		if c.Key.Verification != sum {
			t.Fatalf("%s verification got %q fixture %q", key, c.Key.Verification, sum)
		}
		if c.Key.Architecture != "" || len(c.Licenses) != 0 {
			t.Fatalf("buildinfo has no arch/license: %+v", c)
		}
		oc := oldBy[key]
		if oc.Name == "" {
			t.Fatalf("old dump missing %s", key)
		}
		if oc.Verification != "" {
			t.Fatalf("old dump unexpectedly had verification for %s", key)
		}
		if oc.FromFile[0] != "app" || oc.FromAnalyzer[0] != "go-binary-lang" {
			t.Fatalf("old dump from %+v", oc)
		}
		hits := byComp[c.Key.ID()]
		if len(hits) != 1 {
			t.Fatalf("observations %s %+v", key, hits)
		}
		o := hits[0]
		if o.Component != c.Key.ID() || o.File != "app" || o.Kind != "binary" {
			t.Fatalf("observation %+v", o)
		}
		if o.StartLine != 0 || o.EndLine != 0 {
			t.Fatalf("buildinfo has no source lines: %+v", o)
		}
		if _, ok := obsByID[o.ID()]; !ok {
			t.Fatal("observation id missing")
		}
	}
	bom, _, sbomObs := parseSBOM(t, r)
	var pep *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "github.com/aquasecurity/go-pep440-version" {
			pep = &bom.Components[i]
		}
	}
	if pep == nil {
		t.Fatal("SBOM missing pep440")
	}
	verOK := false
	for _, p := range pep.Properties {
		if p.Name == "sca:verification" && p.Value == want["github.com/aquasecurity/go-pep440-version@v0.0.0-20210121094942-22b2f8951d46"] {
			verOK = true
		}
	}
	if !verOK && (len(pep.Hashes) == 0) {
		t.Fatalf("SBOM lost buildinfo sum: props=%+v hashes=%+v", pep.Properties, pep.Hashes)
	}
	if len(sbomObs) != len(r.Observations) {
		t.Fatalf("SBOM observations %d report %d", len(sbomObs), len(r.Observations))
	}

	repl := scanNamed(t, "gobinary-replace", map[string]string{"app": "analyzer/dep-parser/golang/binary/testdata/replace.elf"})
	rcomps, _, rby := reportIndex(repl)
	mysql := rcomps["github.com/go-sql-driver/mysql@v1.5.0"]
	if mysql.Key.Name == "" {
		t.Fatal("replace module missing")
	}
	if mysql.Key.Source != "github.com/go-sql-driver/mysql" {
		t.Fatalf("replace original path %q", mysql.Key.Source)
	}
	from := rby[mysql.Key.ID()][0].ID()
	found := false
	for _, q := range repl.Requirements {
		if q.From == from && q.Target == "github.com/go-sql-driver/mysql" && q.Constraint == "v0.0.0-00010101000000-000000000000" {
			found = true
		}
	}
	if !found {
		t.Fatalf("original replace version dropped: %s", extra5149JSON(t, repl.Requirements))
	}
	if rcomps["github.com/davecgh/go-spew@v1.1.1"].Key.Name == "" {
		t.Fatal("spew missing")
	}
	nongo := scanReportFiles(t, "gobinary-nongo", map[string]string{"app": "testdata/go_binary/negative-go-binary-bash"})
	if hasName(nongo, "github.com/aquasecurity/go-pep440-version") {
		t.Fatal("non-Go executable produced Go modules")
	}
}

func testFullFieldConan(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/conan/conan")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"ref": "openssl/3.0.5"`) || !strings.Contains(string(raw), `"ref": "zlib/1.2.12"`) {
		t.Fatal("frozen conan refs drifted")
	}
	old := loadOldScan(t, "conan")
	r := scanNamed(t, "conan", map[string]string{"conan.lock": "testdata/conan/conan"})
	comps, obsByID, byComp := reportIndex(r)
	if len(r.Components) != 2 || len(old) != 2 {
		t.Fatalf("conan orig=2 old=%d new=%d", len(old), len(r.Components))
	}
	ssl := comps["openssl@3.0.5"]
	zlib := comps["zlib@1.2.12"]
	if ssl.Key.Name == "" || zlib.Key.Name == "" {
		t.Fatal("missing openssl/zlib")
	}
	if ssl.Key.Source != "openssl/3.0.5" || zlib.Key.Source != "zlib/1.2.12" {
		t.Fatalf("original refs: ssl=%q zlib=%q", ssl.Key.Source, zlib.Key.Source)
	}
	if ssl.Key.Ecosystem != "conan" || ssl.Key.Architecture != "" || ssl.Key.Verification != "" || len(ssl.Licenses) != 0 {
		t.Fatalf("lock has no arch/digest/license: %+v", ssl)
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	if oldBy["openssl@3.0.5"].FromFile[0] != "conan.lock" || oldBy["openssl@3.0.5"].FromAnalyzer[0] != "conan-lang" {
		t.Fatalf("old dump from %+v", oldBy["openssl@3.0.5"])
	}
	if oldBy["openssl@3.0.5"].UpNames[0] != "zlib@1.2.12" {
		t.Fatalf("old dump UpNames %v", oldBy["openssl@3.0.5"].UpNames)
	}
	if oldBy["openssl@3.0.5"].Verification != "" {
		t.Fatal("old dump had verification")
	}
	oh := byComp[ssl.Key.ID()]
	if len(oh) != 1 || oh[0].File != "conan.lock" || oh[0].Kind != "locked" {
		t.Fatalf("openssl observation %+v", oh)
	}
	if oh[0].StartLine != 12 || oh[0].EndLine != 21 {
		t.Fatalf("openssl location %d-%d", oh[0].StartLine, oh[0].EndLine)
	}
	from := oh[0].ID()
	q := requireEdge(t, r, from, "zlib", "1.2.12", "", "zlib/1.2.12")
	if len(q.Resolved) != 1 || obsByID[q.Resolved[0]].Component != zlib.Key.ID() {
		t.Fatalf("zlib resolved %+v", q)
	}
	if strings.Contains(q.Target, "#") || strings.HasPrefix(q.Target, "conan.lock#") {
		t.Fatalf("synthetic target %+v", q)
	}
	bom, sbomReqs, _ := parseSBOM(t, r)
	okReq := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "zlib" && q.Constraint == "1.2.12" && q.Condition == "zlib/1.2.12" {
			okReq = true
		}
	}
	if !okReq {
		t.Fatalf("SBOM lost original zlib ref: %s", extra5149JSON(t, sbomReqs))
	}
	var oc *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "openssl" {
			oc = &bom.Components[i]
		}
	}
	if oc == nil {
		t.Fatal("SBOM missing openssl")
	}
	srcOK := false
	for _, p := range oc.Properties {
		if p.Name == "sca:source" && p.Value == "openssl/3.0.5" {
			srcOK = true
		}
	}
	if !srcOK {
		t.Fatalf("SBOM lost openssl ref: %+v", oc.Properties)
	}

	rev := scanNamed(t, "conan-rev", map[string]string{"conan.lock": "analyzer/dep-parser/c/conan/testdata/happy2.lock"})
	rcomps, _, rby := reportIndex(rev)
	ossl := rcomps["openssl@3.0.3"]
	if ossl.Key.Source != "openssl/3.0.3#288ab73765e69844899535609ee0dfe4" {
		t.Fatalf("revision ref %q", ossl.Key.Source)
	}
	from = rby[ossl.Key.ID()][0].ID()
	requireEdge(t, rev, from, "zlib", "1.2.12", "", "zlib/1.2.12#b76db676bd992afa93dd18a675323942")

	user := scanNamed(t, "conan-user", map[string]string{"conan.lock": "analyzer/dep-parser/c/conan/testdata/happy.lock"})
	ucomps, _, _ := reportIndex(user)
	if ucomps["pkgc@0.1.1"].Key.Source != "pkgc/0.1.1@user/testing" {
		t.Fatalf("user/channel %q", ucomps["pkgc@0.1.1"].Key.Source)
	}
	if ucomps["pkgb@system"].Key.Version != "system" {
		t.Fatalf("system version %q", ucomps["pkgb@system"].Key.Version)
	}
}

func parseDpkgStatus(raw []byte) []origPkg {
	var out []origPkg
	for _, block := range bytes.Split(raw, []byte("\n\n")) {
		h := map[string]string{}
		for _, line := range strings.Split(string(block), "\n") {
			if line == "" || strings.HasPrefix(line, " ") {
				continue
			}
			k, v, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			h[k] = strings.TrimSpace(v)
		}
		if !strings.HasSuffix(h["Status"], " installed") || h["Package"] == "" {
			continue
		}
		p := origPkg{Name: h["Package"], Version: h["Version"], Arch: h["Architecture"]}
		for _, raw := range strings.Split(h["Provides"], ",") {
			if v := strings.TrimSpace(raw); v != "" {
				p.Provides = append(p.Provides, v)
			}
		}
		for _, dep := range strings.Split(h["Depends"], ",") {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if strings.Contains(dep, "|") {
				for _, alt := range strings.Split(dep, "|") {
					name, constraint := dpkgNameVer(alt)
					p.Depends = append(p.Depends, origDep{Target: name, Constraint: constraint, Raw: "or"})
				}
				continue
			}
			name, constraint := dpkgNameVer(dep)
			p.Depends = append(p.Depends, origDep{Target: name, Constraint: constraint, Raw: "and"})
		}
		out = append(out, p)
	}
	return out
}

func dpkgNameVer(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	constraint := "*"
	if i := strings.Index(raw, "("); i >= 0 {
		constraint = strings.TrimSuffix(strings.TrimSpace(raw[i+1:]), ")")
		raw = strings.TrimSpace(raw[:i])
	}
	return raw, constraint
}

func testFullFieldDPKG(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/dpkg/dpkg")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Package: adduser") || !strings.Contains(string(raw), "Depends: passwd, debconf (>= 0.5) | debconf-2.0") {
		t.Fatal("frozen dpkg status drifted")
	}
	if !strings.Contains(string(raw), "Architecture: all") || !strings.Contains(string(raw), "Provides: apt-transport-https (= 2.4.9)") {
		t.Fatal("frozen dpkg architecture/provides drifted")
	}
	orig := parseDpkgStatus(raw)
	old := loadOldScan(t, "dpkg")
	r := scanNamed(t, "dpkg", map[string]string{"var/lib/dpkg/status": "testdata/dpkg/dpkg"})
	comps, obsByID, byComp := reportIndex(r)
	if len(orig) != 10 || len(r.Components) != 10 {
		t.Fatalf("installed dpkg orig=%d new=%d", len(orig), len(r.Components))
	}
	oldReal, oldVirtual := 0, 0
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		if o.Potential {
			oldVirtual++
			if _, ok := comps[o.Name+"@"+o.Version]; ok {
				t.Fatalf("virtual %s@%s promoted to component", o.Name, o.Version)
			}
			continue
		}
		oldReal++
		oldBy[o.Name+"@"+o.Version] = o
	}
	if oldReal != 10 || oldVirtual != 18 {
		t.Fatalf("old dump real=%d virtual=%d", oldReal, oldVirtual)
	}
	for _, p := range orig {
		c, ok := comps[p.Name+"@"+p.Version]
		if !ok {
			t.Fatalf("missing %s@%s", p.Name, p.Version)
		}
		if c.Key.Ecosystem != "dpkg" {
			t.Fatalf("ecosystem %q", c.Key.Ecosystem)
		}
		if c.Key.Architecture != p.Arch {
			t.Fatalf("%s arch got %q fixture %q", p.Name, c.Key.Architecture, p.Arch)
		}
		if c.Key.Verification != "" || c.Key.Source != "" || len(c.Licenses) != 0 {
			t.Fatalf("%s invented digest/source/license %+v", p.Name, c)
		}
		oc := oldBy[p.Name+"@"+p.Version]
		if oc.FromFile[0] != "var/lib/dpkg/status" || oc.FromAnalyzer[0] != "dpkg-pkg" {
			t.Fatalf("old dump from %+v", oc)
		}
		hits := byComp[c.Key.ID()]
		if len(hits) == 0 || hits[0].Kind != "installed" || hits[0].File != "var/lib/dpkg/status" {
			t.Fatalf("observation %+v", hits)
		}
		o := hits[0]
		if o.Component != c.Key.ID() || obsByID[o.ID()].Component != c.Key.ID() {
			t.Fatalf("observation identity %s", p.Name)
		}
		if o.StartLine != 0 || o.EndLine != 0 {
			t.Fatalf("status records have no line numbers: %+v", o)
		}
		gotProv := map[string]bool{}
		for _, s := range o.Provides {
			gotProv[s] = true
		}
		for _, s := range p.Provides {
			if !gotProv[s] {
				t.Fatalf("%s missing provide %q got %v", p.Name, s, o.Provides)
			}
		}
		from := o.ID()
		for _, d := range p.Depends {
			found := false
			for _, q := range r.Requirements {
				if q.From != from || q.Target != d.Target || q.Constraint != d.Constraint {
					continue
				}
				found = true
				if d.Raw == "or" && q.Operator != "or" {
					t.Fatalf("%s alternative %s operator %q", p.Name, d.Target, q.Operator)
				}
				if d.Raw == "and" && q.Operator != "and" {
					t.Fatalf("%s depend %s operator %q", p.Name, d.Target, q.Operator)
				}
				if len(q.Resolved) != 0 {
					t.Fatalf("capability uniquely resolved: %+v", q)
				}
			}
			if !found {
				t.Fatalf("%s missing original Depends %s %s: %s", p.Name, d.Target, d.Constraint, extra5149JSON(t, r.Requirements))
			}
		}
	}
	add := comps["adduser@3.118ubuntu5"]
	from := byComp[add.Key.ID()][0].ID()
	var orGroup string
	for _, q := range r.Requirements {
		if q.From == from && q.Target == "debconf" && q.Constraint == ">= 0.5" && q.Operator == "or" {
			orGroup = q.Group
		}
	}
	if orGroup == "" {
		t.Fatal("debconf alternative lost group")
	}
	foundAlt := false
	for _, q := range r.Requirements {
		if q.From == from && q.Target == "debconf-2.0" && q.Operator == "or" && q.Group == orGroup {
			foundAlt = true
		}
	}
	if !foundAlt {
		t.Fatal("debconf-2.0 alternative dropped")
	}
	bom, sbomReqs, sbomObs := parseSBOM(t, r)
	var ac *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "adduser" {
			ac = &bom.Components[i]
		}
	}
	if ac == nil {
		t.Fatal("SBOM missing adduser")
	}
	archOK := false
	for _, p := range ac.Properties {
		if p.Name == "sca:architecture" && p.Value == "all" {
			archOK = true
		}
	}
	if !archOK {
		t.Fatalf("SBOM architecture %+v", ac.Properties)
	}
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "passwd" && q.Operator == "and" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM lost passwd depend: %s", extra5149JSON(t, sbomReqs))
	}
	if len(sbomObs) != len(r.Observations) {
		t.Fatalf("SBOM observations %d report %d", len(sbomObs), len(r.Observations))
	}
}

func testFullFieldRPM(t *testing.T) {
	// This compact example supplements TestRPMAllFrozenFields, which checks
	// all 129 records and 2535 unique requirements against raw SQLite headers.
	old := loadOldScan(t, "rpm")
	r := scanNamed(t, "rpm", map[string]string{"var/lib/rpm/rpmdb.sqlite": "testdata/rpm/rpmdb.sqlite"})
	comps, obsByID, byComp := reportIndex(r)
	oldReal, oldVirtual := 0, 0
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		if o.Potential {
			oldVirtual++
			if _, ok := comps[o.Name+"@"+o.Version]; ok {
				t.Fatalf("virtual %s@%s promoted to component", o.Name, o.Version)
			}
			continue
		}
		oldReal++
		oldBy[o.Name+"@"+o.Version] = o
	}
	if oldReal != 129 || len(r.Components) != 129 {
		t.Fatalf("installed rpm old=%d new=%d", oldReal, len(r.Components))
	}
	if oldVirtual != 6 {
		t.Fatalf("old virtual %d", oldVirtual)
	}
	for _, name := range []string{"cp", "env", "ln", "mv", "rm", "python3.9"} {
		for _, c := range r.Components {
			if c.Key.Name == name {
				t.Fatalf("path/soname %s invented as component", name)
			}
		}
	}
	c := comps["mariner-release@2.0"]
	if c.Key.Name == "" {
		t.Fatal("missing mariner-release")
	}
	if c.Key.Ecosystem != "rpm" {
		t.Fatalf("ecosystem %q", c.Key.Ecosystem)
	}
	if c.Key.Architecture != "noarch" {
		t.Fatalf("arch %q", c.Key.Architecture)
	}
	if c.Key.Verification != "md5:f7bd337ae2962162ac73a509ed7129f0" {
		t.Fatalf("md5 %q", c.Key.Verification)
	}
	if strings.Join(c.Licenses, ",") != "MIT" {
		t.Fatalf("original license %v", c.Licenses)
	}
	if c.Key.Variant != "epoch=0;release=4.cm2" {
		t.Fatalf("epoch/release %q", c.Key.Variant)
	}
	oc := oldBy["mariner-release@2.0"]
	if oc.Verification != "md5:f7bd337ae2962162ac73a509ed7129f0" {
		t.Fatalf("old dump md5 %+v", oc)
	}
	if oc.FromFile[0] != "var/lib/rpm/rpmdb.sqlite" || oc.FromAnalyzer[0] != "rpm-pkg" {
		t.Fatalf("old dump from %+v", oc)
	}
	hits := byComp[c.Key.ID()]
	if len(hits) != 1 || hits[0].Kind != "installed" || hits[0].File != "var/lib/rpm/rpmdb.sqlite" {
		t.Fatalf("observation %+v", hits)
	}
	o := hits[0]
	if o.Component != c.Key.ID() || obsByID[o.ID()].Component != c.Key.ID() {
		t.Fatal("observation identity")
	}
	provideOK := false
	for _, p := range o.Provides {
		if p == "config(mariner-release) = 2.0-4.cm2" {
			provideOK = true
		}
	}
	if !provideOK {
		t.Fatalf("provide constraint dropped: %v", o.Provides)
	}
	found := false
	for _, q := range r.Requirements {
		if q.From != o.ID() || q.Target != "config(mariner-release)" {
			continue
		}
		found = true
		if q.Constraint != "= 2.0-4.cm2" {
			t.Fatalf("require constraint %q", q.Constraint)
		}
		if q.Condition != "rpmflags=268435464" {
			t.Fatalf("require flags %q", q.Condition)
		}
		if q.Operator != "and" {
			t.Fatalf("operator %q", q.Operator)
		}
		if len(q.Resolved) != 0 {
			t.Fatalf("capability uniquely resolved: %+v", q)
		}
	}
	if !found {
		t.Fatalf("missing config(mariner-release): %s", extra5149JSON(t, r.Requirements))
	}
	for key, oc := range oldBy {
		if _, ok := comps[key]; !ok {
			t.Fatalf("installed identity dropped %s", key)
		}
		if oc.FromAnalyzer[0] != "rpm-pkg" {
			t.Fatalf("old dump analyzer %s", key)
		}
	}
	bom, sbomReqs, sbomObs := parseSBOM(t, r)
	var mr *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "mariner-release" {
			mr = &bom.Components[i]
		}
	}
	if mr == nil {
		t.Fatal("SBOM missing mariner-release")
	}
	if len(mr.Hashes) == 0 || mr.Hashes[0].Value != "f7bd337ae2962162ac73a509ed7129f0" {
		t.Fatalf("SBOM md5 %+v", mr.Hashes)
	}
	ok := false
	for _, q := range sbomReqs {
		if q.From == o.ID() && q.Target == "config(mariner-release)" && q.Constraint == "= 2.0-4.cm2" && q.Condition == "rpmflags=268435464" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM lost require flags: %s", extra5149JSON(t, sbomReqs))
	}
	if len(sbomObs) != len(r.Observations) {
		t.Fatalf("SBOM observations %d report %d", len(sbomObs), len(r.Observations))
	}
}

func testFullFieldPip(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/python_pip/requirements.txt")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "click==8.0.0") || !strings.Contains(text, "Jinja2<3.0.0") || !strings.Contains(text, "MarkupSafe>2.0.0") || !strings.HasSuffix(strings.TrimSpace(text), "Werkzeug") {
		t.Fatalf("frozen requirements drifted: %q", text)
	}
	old := loadOldScan(t, "pip")
	r := scanNamed(t, "pip", map[string]string{"requirements.txt": "testdata/python_pip/requirements.txt"})
	comps, _, byComp := reportIndex(r)
	if len(r.Components) != 6 {
		t.Fatalf("declared lines new=%d", len(r.Components))
	}
	if len(old) != 3 {
		t.Fatalf("old dump of a successful scan has only == pins, got %d", len(old))
	}
	for _, o := range old {
		if o.Name != "Flask" && o.Name != "click" && o.Name != "itsdangerous" {
			t.Fatalf("old dump included a range as installed: %+v", o)
		}
		if o.FromFile[0] != "requirements.txt" || o.FromAnalyzer[0] != "python-pip-lang" {
			t.Fatalf("old dump from %+v", o)
		}
	}
	exact := map[string]string{"Flask": "2.0.0", "click": "8.0.0", "itsdangerous": "2.0.0"}
	for name, ver := range exact {
		c, ok := comps[name+"@"+ver]
		if !ok {
			t.Fatalf("missing exact pin %s@%s", name, ver)
		}
		if c.Key.Ecosystem != "pypi" || c.Key.Architecture != "" || len(c.Licenses) != 0 || c.Key.Verification != "" {
			t.Fatalf("%s invented arch/license/digest %+v", name, c)
		}
		o := byComp[c.Key.ID()][0]
		if o.Kind != "declared" || o.File != "requirements.txt" {
			t.Fatalf("observation %+v", o)
		}
		found := false
		for _, q := range r.Requirements {
			if q.From == o.ID() && q.Target == name && q.Constraint == "=="+ver {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s original == pin dropped: %s", name, extra5149JSON(t, r.Requirements))
		}
	}
	for _, name := range []string{"Jinja2", "MarkupSafe", "Werkzeug"} {
		c, ok := comps[name+"@"]
		if !ok {
			t.Fatalf("range/unpinned %s promoted or dropped: %s", name, extra5149JSON(t, r.Components))
		}
		if c.Key.Version != "" {
			t.Fatalf("%s version promoted %q", name, c.Key.Version)
		}
		o := byComp[c.Key.ID()][0]
		want := map[string]string{"Jinja2": "<3.0.0", "MarkupSafe": ">2.0.0", "Werkzeug": ""}
		found := false
		for _, q := range r.Requirements {
			if q.From == o.ID() && q.Target == name && q.Constraint == want[name] {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s original constraint dropped: %s", name, extra5149JSON(t, r.Requirements))
		}
	}
	if byComp[comps["click@8.0.0"].Key.ID()][0].StartLine != 1 || byComp[comps["Flask@2.0.0"].Key.ID()][0].StartLine != 2 {
		t.Fatalf("requirement line numbers: click=%d flask=%d", byComp[comps["click@8.0.0"].Key.ID()][0].StartLine, byComp[comps["Flask@2.0.0"].Key.ID()][0].StartLine)
	}
	bom, sbomReqs, _ := parseSBOM(t, r)
	var fl *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "Flask" && bom.Components[i].Version == "2.0.0" {
			fl = &bom.Components[i]
		}
	}
	if fl == nil {
		t.Fatal("SBOM missing Flask pin")
	}
	ok := false
	from := byComp[comps["Jinja2@"].Key.ID()][0].ID()
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "Jinja2" && q.Constraint == "<3.0.0" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM lost Jinja2 range: %s", extra5149JSON(t, sbomReqs))
	}
}

func testFullFieldPipenv(t *testing.T) {
	raw, err := fixtures.ReadFile("testdata/python_pipenv/Pipfile.lock")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"pytz"`) || !strings.Contains(text, `"version": "==2022.7.1"`) {
		t.Fatal("frozen Pipfile.lock identity drifted")
	}
	if !strings.Contains(text, "sha256:01a0681c4b9684a28304615eba55d1ab31ae00bf68ec157ec3708a8182dbbcd0") || !strings.Contains(text, `"index": "pypi"`) {
		t.Fatal("frozen Pipfile.lock hash/index drifted")
	}
	if !strings.Contains(text, `"url": "https://pypi.org/simple"`) || !strings.Contains(text, `"pipfile-spec": 6`) {
		t.Fatal("frozen Pipfile.lock sources table drifted")
	}
	oldRaw, err := fixtures.ReadFile("testdata/full_field/old-scanfilesystem-pipenv.json")
	if err != nil {
		t.Fatal(err)
	}
	var old []oldScanRec
	if err := json.Unmarshal(oldRaw, &old); err != nil {
		t.Fatal(err)
	}
	if len(old) != 0 {
		t.Fatalf("old matcher used requirements.txt and produced no Pipfile.lock packages, got %d", len(old))
	}
	r := scanNamed(t, "pipenv", map[string]string{"Pipfile.lock": "testdata/python_pipenv/Pipfile.lock"})
	comps, _, byComp := reportIndex(r)
	c, ok := comps["pytz@2022.7.1"]
	if !ok || len(r.Components) != 1 {
		t.Fatalf("default category identity: %s", extra5149JSON(t, r.Components))
	}
	if c.Key.Ecosystem != "pypi" {
		t.Fatalf("ecosystem %q", c.Key.Ecosystem)
	}
	if c.Key.Source != "pypi" {
		t.Fatalf("index source %q (package record index, not _meta URL expansion)", c.Key.Source)
	}
	if !strings.Contains(c.Key.Verification, "sha256:01a0681c4b9684a28304615eba55d1ab31ae00bf68ec157ec3708a8182dbbcd0") || !strings.Contains(c.Key.Verification, "sha256:78f4f37d8198e0627c5f1143240bb0206b8691d8d7ac6d78fee88b78733f8c4a") {
		t.Fatalf("hashes %q", c.Key.Verification)
	}
	if c.Key.Architecture != "" || len(c.Licenses) != 0 {
		t.Fatalf("lock has no arch/license %+v", c)
	}
	o := byComp[c.Key.ID()][0]
	if o.Kind != "locked" || o.File != "Pipfile.lock" || o.StartLine != 17 || o.EndLine != 24 {
		t.Fatalf("observation %+v", o)
	}
	if o.Scope != "" {
		t.Fatalf("default group is not a scope label: %q", o.Scope)
	}
	if len(r.Requirements) != 0 {
		t.Fatalf("Pipfile.lock default entries have no require graph: %s", extra5149JSON(t, r.Requirements))
	}
	bom, sbomReqs, _ := parseSBOM(t, r)
	var pz *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "pytz" {
			pz = &bom.Components[i]
		}
	}
	if pz == nil {
		t.Fatal("SBOM missing pytz")
	}
	srcOK := false
	for _, p := range pz.Properties {
		if p.Name == "sca:source" && p.Value == "pypi" {
			srcOK = true
		}
	}
	if !srcOK {
		t.Fatalf("SBOM lost package index source: props=%+v", pz.Properties)
	}
	wantHashes := map[string]bool{
		"01a0681c4b9684a28304615eba55d1ab31ae00bf68ec157ec3708a8182dbbcd0": false,
		"78f4f37d8198e0627c5f1143240bb0206b8691d8d7ac6d78fee88b78733f8c4a": false,
	}
	for _, h := range pz.Hashes {
		if _, ok := wantHashes[h.Value]; ok && (h.Algorithm == "SHA-256" || h.Algorithm == "sha256") {
			wantHashes[h.Value] = true
		}
	}
	for value, ok := range wantHashes {
		if !ok {
			t.Fatalf("SBOM lost sha256 %s: hashes=%+v", value, pz.Hashes)
		}
	}
	if len(sbomReqs) != 0 {
		t.Fatalf("SBOM invented pipenv edges: %s", extra5149JSON(t, sbomReqs))
	}
}

func testFullFieldComposer(t *testing.T) {
	lock, err := fixtures.ReadFile("testdata/php_composer/positive/composer.lock")
	if err != nil {
		t.Fatal(err)
	}
	man, err := fixtures.ReadFile("testdata/php_composer/positive/composer.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lock), `"name": "pear/log"`) || !strings.Contains(string(lock), `"version": "1.13.3"`) {
		t.Fatal("frozen composer.lock identity drifted")
	}
	if !strings.Contains(string(lock), `"url": "https://github.com/pear/Log.git"`) || !strings.Contains(string(lock), `"pear/pear_exception": "1.0.1 || 1.0.2"`) {
		t.Fatal("frozen composer.lock source/require drifted")
	}
	if !strings.Contains(string(man), `"pear/log": "^1.13"`) {
		t.Fatal("frozen composer.json require drifted")
	}
	old := loadOldScan(t, "composer")
	r := scanNamed(t, "composer", map[string]string{
		"composer.lock": "testdata/php_composer/positive/composer.lock",
		"composer.json": "testdata/php_composer/positive/composer.json",
	})
	comps, _, byComp := reportIndex(r)
	if len(old) != 2 {
		t.Fatalf("old dump ignored composer.json, got %d", len(old))
	}
	if len(r.Components) != 3 {
		t.Fatalf("lock 2 plus json declaration: new=%d", len(r.Components))
	}
	logc := comps["pear/log@1.13.3"]
	exc := comps["pear/pear_exception@v1.0.2"]
	decl := comps["pear/log@"]
	if logc.Key.Name == "" || exc.Key.Name == "" || decl.Key.Name == "" {
		t.Fatalf("identities: %s", extra5149JSON(t, r.Components))
	}
	if logc.Key.Source != "https://github.com/pear/Log.git#21af0be11669194d72d88b5ee9d5f176dc75d9a3" {
		t.Fatalf("source %q", logc.Key.Source)
	}
	if strings.Join(logc.Licenses, ",") != "MIT" || strings.Join(exc.Licenses, ",") != "BSD-2-Clause" {
		t.Fatalf("licenses %v %v", logc.Licenses, exc.Licenses)
	}
	if logc.Key.Verification != "" {
		t.Fatalf("empty dist shasum invented digest %q", logc.Key.Verification)
	}
	if logc.Key.Architecture != "" {
		t.Fatalf("autoload/dist are not architecture: %q", logc.Key.Architecture)
	}
	oldBy := map[string]oldScanRec{}
	for _, o := range old {
		oldBy[o.Name+"@"+o.Version] = o
	}
	if oldBy["pear/log@1.13.3"].FromFile[0] != "composer.lock" || oldBy["pear/log@1.13.3"].License[0] != "MIT" {
		t.Fatalf("old dump %+v", oldBy["pear/log@1.13.3"])
	}
	if oldBy["pear/log@1.13.3"].UpNames[0] != "pear/pear_exception@v1.0.2" {
		t.Fatalf("old dump UpNames %v", oldBy["pear/log@1.13.3"].UpNames)
	}
	o := byComp[logc.Key.ID()][0]
	if o.Kind != "locked" || o.File != "composer.lock" || o.StartLine != 9 {
		t.Fatalf("lock observation %+v", o)
	}
	q := requireEdge(t, r, o.ID(), "pear/pear_exception", "1.0.1 || 1.0.2", "", "")
	if len(q.Resolved) != 1 {
		t.Fatalf("unique lock require should resolve: %+v", q)
	}
	phpOK := false
	for _, q := range r.Requirements {
		if q.From == o.ID() && q.Target == "php" && q.Constraint == ">5.2" {
			phpOK = true
			if len(q.Resolved) != 0 {
				t.Fatalf("php platform uniquely resolved: %+v", q)
			}
		}
	}
	if !phpOK {
		t.Fatalf("php platform require dropped: %s", extra5149JSON(t, r.Requirements))
	}
	do := byComp[decl.Key.ID()][0]
	if do.Kind != "declared" || do.File != "composer.json" {
		t.Fatalf("manifest observation %+v", do)
	}
	foundJSON := false
	for _, q := range r.Requirements {
		if q.From == do.ID() && q.Target == "pear/log" && q.Constraint == "^1.13" {
			foundJSON = true
			if len(q.Resolved) != 0 {
				t.Fatalf("json range uniquely resolved to lock by scan order: %+v", q)
			}
		}
	}
	if !foundJSON {
		t.Fatalf("composer.json original range dropped: %s", extra5149JSON(t, r.Requirements))
	}
	if decl.Key.Version != "" {
		t.Fatalf("json ^1.13 promoted to lock version %q", decl.Key.Version)
	}
	bom, sbomReqs, _ := parseSBOM(t, r)
	var pl *dxtypes.BOMComponent
	for i := range bom.Components {
		if bom.Components[i].Name == "pear/log" && bom.Components[i].Version == "1.13.3" {
			pl = &bom.Components[i]
		}
	}
	if pl == nil {
		t.Fatal("SBOM missing locked pear/log")
	}
	srcOK := false
	for _, p := range pl.Properties {
		if p.Name == "sca:source" && strings.Contains(p.Value, "github.com/pear/Log.git") {
			srcOK = true
		}
	}
	if !srcOK {
		t.Fatalf("SBOM lost git source: %+v", pl.Properties)
	}
	ok := false
	for _, q := range sbomReqs {
		if q.From == o.ID() && q.Target == "pear/pear_exception" && q.Constraint == "1.0.1 || 1.0.2" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM lost original require range: %s", extra5149JSON(t, sbomReqs))
	}
}
