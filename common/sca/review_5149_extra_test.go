package sca

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
)

func extra5149Scan(t *testing.T, files map[string]string, opts ...ScanOption) *model.Report {
	t.Helper()
	input := fstest.MapFS{}
	for name, data := range files {
		input[name] = &fstest.MapFile{Data: []byte(data), Mode: 0444}
	}
	options := append([]ScanOption{WithSnapshotID("review-5149-extra")}, opts...)
	r, err := ScanReport(context.Background(), input, options...)
	if r == nil {
		t.Fatalf("nil report: %v", err)
	}
	return r
}

func extra5149JSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReview5149_BOMDiamondAndCancel(t *testing.T) {
	bom := func(name, imports string) string {
		return `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>` + name + `</artifactId><version>1.0</version><packaging>pom</packaging><dependencyManagement><dependencies>` + imports + `</dependencies></dependencyManagement></project>`
	}
	imp := func(name string) string {
		return `<dependency><groupId>org.review</groupId><artifactId>` + name + `</artifactId><version>1.0</version><type>pom</type><scope>import</scope></dependency>`
	}
	files := map[string]string{
		"pom.xml": `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>app</artifactId><version>1.0</version><dependencyManagement><dependencies>` + imp("left") + imp("right") + `</dependencies></dependencyManagement><dependencies><dependency><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version></dependency></dependencies></project>`,
		"repository/org/review/left/1.0/left-1.0.pom":     bom("left", imp("shared")),
		"repository/org/review/right/1.0/right-1.0.pom":   bom("right", imp("shared")),
		"repository/org/review/shared/1.0/shared-1.0.pom": `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>shared</artifactId><version>1.0</version><packaging>pom</packaging><dependencyManagement><dependencies><dependency><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version></dependency></dependencies></dependencyManagement></project>`,
		"repository/org/review/dep/1.0/dep-1.0.pom":       `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version></project>`,
	}
	r := extra5149Scan(t, files)
	if !r.Complete {
		t.Fatalf("shared BOM diamond treated as a cycle: %s", extra5149JSON(t, r))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input := fstest.MapFS{"pom.xml": {Data: []byte(files["pom.xml"])}}
	cr, err := ScanReport(ctx, input)
	if err == nil || cr.Complete {
		t.Fatal("cancelled scan reported complete")
	}
	for _, d := range cr.Diagnostics {
		if d.Code == "cancelled" {
			return
		}
	}
	t.Fatalf("cancel not classified: %s", extra5149JSON(t, cr))
}

func TestReview5149_BOMDeepAndBudgetChild(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"deep", "storm"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestReview5149_BOMDeepAndBudgetChildWorker$", "-test.count=1")
			cmd.Env = append(os.Environ(), "YAK_SCA_REVIEW5149_EXTRA="+mode)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("bounded BOM graph did not finish: %v", ctx.Err())
			}
			if err != nil {
				t.Fatalf("child failed: %v\n%s", err, output)
			}
		})
	}
}

func TestReview5149_BOMDeepAndBudgetChildWorker(t *testing.T) {
	mode := os.Getenv("YAK_SCA_REVIEW5149_EXTRA")
	if mode == "" {
		t.Skip("child-only graph bound")
	}
	debug.SetMaxStack(512 << 10)
	debug.SetMemoryLimit(64 << 20)
	imp := func(name string) string {
		return `<dependency><groupId>org.review</groupId><artifactId>` + name + `</artifactId><version>1.0</version><type>pom</type><scope>import</scope></dependency>`
	}
	bom := func(name, target string) string {
		body := ""
		if target != "" {
			body = imp(target)
		}
		return `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>` + name + `</artifactId><version>1.0</version><packaging>pom</packaging><dependencyManagement><dependencies>` + body + `</dependencies></dependencyManagement></project>`
	}
	files := map[string]string{
		"pom.xml": `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>app</artifactId><version>1.0</version><dependencyManagement><dependencies>` + imp("n0") + `</dependencies></dependencyManagement><dependencies><dependency><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version></dependency></dependencies></project>`,
		"repository/org/review/dep/1.0/dep-1.0.pom": `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version></project>`,
	}
	limits := ResourceLimits{MaxReferenceDepth: 8, MaxResolveSteps: 64}
	if mode == "deep" {
		for i := 0; i < 20; i++ {
			next := ""
			if i < 19 {
				next = fmt.Sprintf("n%d", i+1)
			}
			name := fmt.Sprintf("n%d", i)
			files[fmt.Sprintf("repository/org/review/%s/1.0/%s-1.0.pom", name, name)] = bom(name, next)
		}
	} else {
		var imports strings.Builder
		for i := 0; i < 12; i++ {
			imports.WriteString(imp("shared"))
		}
		files["repository/org/review/n0/1.0/n0-1.0.pom"] = `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>n0</artifactId><version>1.0</version><packaging>pom</packaging><dependencyManagement><dependencies>` + imports.String() + `</dependencies></dependencyManagement></project>`
		files["repository/org/review/shared/1.0/shared-1.0.pom"] = bom("shared", "")
		limits.MaxResolveSteps = 8
	}
	r, err := ScanReport(context.Background(), extraFS(files), WithSnapshotID("review-5149-extra"), WithResourceLimits(limits))
	if r == nil || err == nil || r.Complete {
		t.Fatalf("deep/storm must finish as a bounded incomplete result: report=%+v err=%v", r, err)
	}
}

func extraFS(files map[string]string) fstest.MapFS {
	input := fstest.MapFS{}
	for name, data := range files {
		input[name] = &fstest.MapFile{Data: []byte(data), Mode: 0444}
	}
	return input
}

func TestReview5149_FillReportPermutationAndWorkers(t *testing.T) {
	makePkgs := func() []*dxtypes.Package {
		var pkgs []*dxtypes.Package
		for i := 0; i < 8; i++ {
			p := &dxtypes.Package{Name: fmt.Sprintf("demo%d", i), Version: "1.0.0", Verification: ""}
			p.EnsureDetails()
			p.Ecosystem = "npm"
			p.Instance = fmt.Sprintf("node_modules/demo%d", i)
			p.Snapshot = "fixed"
			p.ProjectRoot = "."
			pkgs = append(pkgs, p)
		}
		return pkgs
	}
	permute := func(in []*dxtypes.Package, seed int) []*dxtypes.Package {
		out := append([]*dxtypes.Package(nil), in...)
		for i := range out {
			j := (i*3 + seed) % len(out)
			out[i], out[j] = out[j], out[i]
		}
		return out
	}
	var baseline string
	src := makePkgs()
	for seed := 0; seed < 32; seed++ {
		r := &model.Report{Complete: true}
		fillReport(r, permute(src, seed), ResourceLimits{MaxComponents: 1, MaxObservations: 400000, MaxEdges: 1000000})
		r.Normalize()
		got := extra5149JSON(t, r)
		if seed == 0 {
			baseline = got
		} else if got != baseline {
			t.Fatalf("fillReport permutation changed truncated report\n%s\n%s", baseline, got)
		}
	}
	lock := `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0"}`
	for i := 0; i < 8; i++ {
		lock += fmt.Sprintf(`,"node_modules/demo%d":{"version":"1.0.0"}`, i)
	}
	lock += `}}`
	var want string
	for _, workers := range []int{1, 2, 4, 8} {
		r, err := ScanReport(context.Background(), extraFS(map[string]string{"package-lock.json": lock}), _withConcurrent(workers), WithSnapshotID("fixed"), WithResourceLimits(ResourceLimits{MaxComponents: 1}))
		if r == nil || err == nil || r.Complete {
			t.Fatalf("workers=%d expected incomplete: %v", workers, err)
		}
		got := extra5149JSON(t, r)
		if want == "" {
			want = got
		} else if got != want {
			t.Fatalf("worker count changed truncated report")
		}
	}
}

func TestReview5149_NPMRootRequirementVariants(t *testing.T) {
	t.Run("lock_optional_dev_missing", func(t *testing.T) {
		r, err := ScanReport(context.Background(), extraFS(map[string]string{
			"package-lock.json": `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0","dependencies":{"demo":"^1.0.0","gone":"^2.0.0"},"optionalDependencies":{"opt":"^1.0.0"},"devDependencies":{"dev":"^1.0.0"}},"node_modules/demo":{"version":"1.2.0"},"node_modules/opt":{"version":"1.0.1"},"node_modules/dev":{"version":"1.0.2"}}}`,
		}), WithSnapshotID("fixed"))
		if err != nil || !r.Complete {
			t.Fatalf("valid lock failed: %v %s", err, extra5149JSON(t, r))
		}
		found := map[string]model.Requirement{}
		for _, q := range r.Requirements {
			if q.Target == "demo" || q.Target == "opt" || q.Target == "dev" || q.Target == "gone" {
				found[q.Target+"|"+q.Scope] = q
			}
		}
		if found["demo|runtime"].Constraint != "^1.0.0" || len(found["demo|runtime"].Resolved) == 0 {
			t.Fatalf("runtime root declaration lost: %s", extra5149JSON(t, found))
		}
		if found["opt|optional"].Constraint != "^1.0.0" || found["dev|dev"].Constraint != "^1.0.0" {
			t.Fatalf("optional/dev scope lost: %s", extra5149JSON(t, found))
		}
		if found["gone|runtime"].Constraint != "^2.0.0" || len(found["gone|runtime"].Resolved) != 0 {
			t.Fatalf("missing target must stay unresolved: %s", extra5149JSON(t, found))
		}
	})
	t.Run("manifest_only", func(t *testing.T) {
		r := extra5149Scan(t, map[string]string{"package.json": `{"name":"app","version":"1.0.0","dependencies":{"demo":"^1.0.0"}}`})
		ok := false
		for _, q := range r.Requirements {
			if q.Target == "demo" && q.Constraint == "^1.0.0" {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("manifest declaration lost: %s", extra5149JSON(t, r))
		}
	})
	t.Run("lock_and_manifest", func(t *testing.T) {
		r := extra5149Scan(t, map[string]string{
			"package.json":      `{"name":"app","version":"1.0.0","dependencies":{"demo":"^1.0.0"}}`,
			"package-lock.json": `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0","dependencies":{"demo":"^1.0.0"}},"node_modules/demo":{"version":"1.2.0"}}}`,
		})
		ok := false
		for _, q := range r.Requirements {
			if q.Target == "demo" && q.Constraint == "^1.0.0" && len(q.Resolved) > 0 {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("combined lock/manifest lost original constraint: %s", extra5149JSON(t, r))
		}
	})
}

func TestReview5149_DigestEvidenceReportAndSBOM(t *testing.T) {
	sum := sha512.Sum512([]byte("payload"))
	b64 := base64.StdEncoding.EncodeToString(sum[:])
	hexed := hex.EncodeToString(sum[:])
	sri := "sha512-" + b64
	alt := "sha512:" + hexed
	lock := func(integrity string, extra string) string {
		return `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0","dependencies":{"demo":"1.2.0"}},"node_modules/demo":{"name":"demo","version":"1.2.0","resolved":"https://example.invalid/demo.tgz","integrity":"` + integrity + `"}` + extra + `}}`
	}
	t.Run("encoding_variant", func(t *testing.T) {
		a := extra5149Scan(t, map[string]string{"package-lock.json": lock(sri, "")})
		b := extra5149Scan(t, map[string]string{"package-lock.json": lock(alt, "")})
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
			t.Fatalf("same digest encodings diverged: %q %q", va, vb)
		}
		bom := dxtypes.CreateCycloneDXSBOMFromReport(a)
		found := false
		for _, c := range bom.Components {
			if c.Name != "demo" {
				continue
			}
			for _, h := range c.Hashes {
				if h.Algorithm == "SHA-512" && strings.EqualFold(h.Value, hexed) {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("SBOM dropped declared hash: %s", extra5149JSON(t, bom.Components))
		}
	})
	t.Run("missing", func(t *testing.T) {
		r := extra5149Scan(t, map[string]string{"package-lock.json": lock("", "")})
		for _, c := range r.Components {
			if c.Key.Name == "demo" && c.Key.Verification != "" {
				t.Fatal("invented digest")
			}
		}
	})
	t.Run("illegal_and_multiple", func(t *testing.T) {
		r := extra5149Scan(t, map[string]string{"package-lock.json": lock("sha512-%%%% sha1-"+base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 20)), "")})
		var ver string
		for _, c := range r.Components {
			if c.Key.Name == "demo" {
				ver = c.Key.Verification
			}
		}
		if !strings.Contains(ver, "sha1:") {
			t.Fatalf("valid digest dropped with illegal sibling: %s", extra5149JSON(t, r))
		}
		if strings.Contains(ver, "%%%%") {
			t.Fatal("illegal encoding kept as identity")
		}
	})
	t.Run("pnpm_and_cargo", func(t *testing.T) {
		pnpm := extra5149Scan(t, map[string]string{"pnpm-lock.yaml": "lockfileVersion: '6.0'\npackages:\n  /demo@1.0.0:\n    resolution: {integrity: " + sri + "}\n    dev: false\n"})
		ok := false
		for _, c := range pnpm.Components {
			if c.Key.Name == "demo" && c.Key.Verification != "" {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("pnpm integrity dropped: %s", extra5149JSON(t, pnpm))
		}
		cargo := extra5149Scan(t, map[string]string{"Cargo.lock": "version = 3\n[[package]]\nname = 'demo'\nversion = '1.0.0'\nsource = 'registry+https://example.invalid/index'\nchecksum = '" + strings.Repeat("ab", 32) + "'\n"})
		ok = false
		for _, c := range cargo.Components {
			if c.Key.Name == "demo" && strings.HasPrefix(c.Key.Verification, "sha256:") {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("cargo checksum dropped: %s", extra5149JSON(t, cargo))
		}
	})
}

func TestReview5149_SchemaLegalAndMoreInvalid(t *testing.T) {
	legal := []struct{ file, text string }{
		{"pnpm-lock.yaml", "lockfileVersion: 5.4\npackages:\n  /demo/1.0.0:\n    dev: false\n"},
		{"pnpm-lock.yaml", "lockfileVersion: '6.0'\npackages:\n  /demo@1.0.0:\n    dev: false\n"},
		{"Cargo.lock", "[[package]]\nname = 'demo'\nversion = '1.0.0'\n"},
		{"Cargo.lock", "version = 3\n[[package]]\nname = 'demo'\nversion = '1.0.0'\n"},
	}
	for i, tc := range legal {
		r, err := ScanReport(context.Background(), extraFS(map[string]string{tc.file: tc.text}), WithSnapshotID("fixed"))
		if err != nil || r == nil || !r.Complete {
			t.Fatalf("legal[%d] rejected: %v %s", i, err, extra5149JSON(t, r))
		}
	}
	invalid := []struct{ file, text string }{
		{"pnpm-lock.yaml", "lockfileVersion: Inf\npackages:\n  /demo@1.0.0:\n    name: demo\n    version: 1.0.0\n"},
		{"pnpm-lock.yaml", "lockfileVersion: true\npackages:\n  /demo@1.0.0:\n    name: demo\n    version: 1.0.0\n"},
		{"Cargo.lock", "version = 4\n[[package]]\nname = 'demo'\nversion = '1.0.0'\n"},
	}
	for i, tc := range invalid {
		r, err := ScanReport(context.Background(), extraFS(map[string]string{tc.file: tc.text}), WithSnapshotID("fixed"))
		if r == nil || err == nil || r.Complete {
			t.Fatalf("invalid[%d] accepted: %v %+v", i, err, r)
		}
		for _, c := range r.Components {
			if c.Key.Name == "" {
				t.Fatal("anonymous component")
			}
		}
	}
}

func TestReview5149_POMUnknownVersionVariants(t *testing.T) {
	r := extra5149Scan(t, map[string]string{
		"pom.xml": `<project><modelVersion>4.0.0</modelVersion><parent><groupId>org.review</groupId><artifactId>missing-parent</artifactId><version>1.0</version></parent><groupId>org.review</groupId><artifactId>app</artifactId><version>1.0</version><dependencies><dependency><groupId>org.review</groupId><artifactId>dep</artifactId><version>${missing}</version></dependency></dependencies></project>`,
	})
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
		if obs[q.From] == "org.review:app" && (q.Target == "org.review:dep" || strings.Contains(q.Target, "org.review:dep")) {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("unknown property/parent dropped app->dep: %s", extra5149JSON(t, r))
	}
}

func TestReview5149_ErrorClassification(t *testing.T) {
	r, err := ScanReport(context.Background(), extraFS(map[string]string{"a\\b": "x"}), WithSnapshotID("fixed"))
	if err == nil {
		t.Fatal("invalid path accepted")
	}
	seen := false
	for _, d := range r.Diagnostics {
		if d.Code == "invalid_path" {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("invalid_path not classified: %s", extra5149JSON(t, r))
	}
}
