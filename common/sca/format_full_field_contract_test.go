package sca

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
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

type origDep struct{ Target, Constraint string }

type origPkg struct {
	Name, Version, Verification, Source, Arch, LicenseRaw, Integrity, ResolvedURL string
	Provides                                                                      []string
	Depends                                                                       []origDep
	Start, End                                                                    int
	Dev                                                                           bool
	Indirect                                                                      bool
	DeclaredPath, DeclaredVersion                                                 string
}

func scanNamed(t *testing.T, id string, files map[string]string) *model.Report {
	t.Helper()
	in := fstest.MapFS{}
	for path, file := range files {
		raw, err := os.ReadFile(file)
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
	raw, err := os.ReadFile("testdata/full_field/old-scanfilesystem-" + format + ".json")
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
					p.Depends = append(p.Depends, origDep{name, c})
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
			cur.Depends = append(cur.Depends, origDep{f[0], c})
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
				p.Depends = append(p.Depends, origDep{tgt, c})
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
}

func testFullFieldAPK(t *testing.T) {
	raw, err := os.ReadFile("testdata/apk/apk")
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
	raw, err := os.ReadFile("testdata/rust_cargo/positive/Cargo.lock")
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
				if d.Constraint != "" && q.Constraint != d.Constraint {
					t.Fatalf("%s dep %s constraint got %q fixture %q", p.Name, d.Target, q.Constraint, d.Constraint)
				}
				if len(q.Resolved) != 1 {
					t.Fatalf("%s dep %s resolved %v", p.Name, d.Target, q.Resolved)
				}
				res := obsByID[q.Resolved[0]]
				want := comps[d.Target+"@"+d.Constraint]
				if d.Constraint != "" && res.Component != want.Key.ID() {
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
		if q.From == from && q.Target == "regex" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM app->regex not associated: %s", extra5149JSON(t, sbomReqs))
	}
}

func testFullFieldNPM(t *testing.T) {
	raw, err := os.ReadFile("testdata/node_npm/positive_folder/package-lock.json")
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
	mod, err := os.ReadFile("testdata/go_mod/positive/mod")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile("testdata/go_mod/positive/sum")
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
