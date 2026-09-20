// Review regressions for yaklang/yaklang PR #5149.
// Audited head: 31a4ae11143659b1336540e6f5d68fedde729b6f.
// Authored from source review. Not compiled or run against the PR in this environment.
// Copy into common/sca/review_5149_regression_test.go in the checked-out PR.
package sca_test

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/sca/model"
)

func review5149Scan(files map[string]string, opts ...sca.ScanOption) (*model.Report, error) {
	input := fstest.MapFS{}
	for name, data := range files {
		input[name] = &fstest.MapFile{Data: []byte(data), Mode: 0444}
	}
	options := []sca.ScanOption{sca.WithSnapshotID("review-5149-fixed")}
	options = append(options, opts...)
	return sca.ScanReport(context.Background(), input, options...)
}

func review5149JSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// R01: run a possibly fatal recursion regression only in a guarded child.
// The SCA runtime itself must remain free of process-launching functionality.
func TestReview5149_CyclicBOM(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"self", "two-node"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestReview5149_CyclicBOMChild$", "-test.count=1")
			cmd.Env = append(os.Environ(), "YAK_SCA_REVIEW5149_CHILD="+mode)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("BOM cycle did not terminate within the guard deadline: %v", ctx.Err())
			}
			if err != nil {
				t.Fatalf("BOM cycle must return a structured error, not crash: %v\n%s", err, output)
			}
		})
	}
}

func TestReview5149_CyclicBOMChild(t *testing.T) {
	mode := os.Getenv("YAK_SCA_REVIEW5149_CHILD")
	if mode == "" {
		t.Skip("child-only recursion regression")
	}
	// An unbounded recursion must fail this child without exhausting the test host.
	debug.SetMaxStack(512 << 10)
	debug.SetMemoryLimit(64 << 20)
	importBOM := func(name string) string {
		return `<dependency><groupId>org.review</groupId><artifactId>` + name + `</artifactId><version>1.0</version><type>pom</type><scope>import</scope></dependency>`
	}
	bom := func(name, target string) string {
		return `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>` + name + `</artifactId><version>1.0</version><packaging>pom</packaging><dependencyManagement><dependencies>` + importBOM(target) + `</dependencies></dependencyManagement></project>`
	}
	root := `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>app</artifactId><version>1.0</version><dependencyManagement><dependencies>` + importBOM("bom-a") + `</dependencies></dependencyManagement><dependencies><dependency><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version></dependency></dependencies></project>`
	target := "bom-a"
	if mode == "two-node" {
		target = "bom-b"
	}
	files := map[string]string{
		"pom.xml": root,
		"repository/org/review/bom-a/1.0/bom-a-1.0.pom": bom("bom-a", target),
		"repository/org/review/dep/1.0/dep-1.0.pom":     `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>dep</artifactId><version>1.0</version></project>`,
	}
	if mode == "two-node" {
		files["repository/org/review/bom-b/1.0/bom-b-1.0.pom"] = bom("bom-b", "bom-a")
	}
	r, err := review5149Scan(files, sca.WithResourceLimits(sca.ResourceLimits{MaxReferenceDepth: 8, MaxResolveSteps: 64}))
	if r == nil || err == nil || r.Complete {
		t.Fatalf("cycle must be diagnosed as incomplete: report=%+v err=%v", r, err)
	}
	text := strings.ToLower(err.Error())
	if !strings.Contains(text, "cycle") && !strings.Contains(text, "circular") {
		t.Fatalf("expected an explicit cycle diagnosis, got %v", err)
	}
}

// R02: Normalize after truncation cannot restore a deterministic selection.
func TestReview5149_ComponentLimitDeterminism(t *testing.T) {
	packages := map[string]any{"": map[string]any{"name": "app", "version": "1.0.0"}}
	for i := 0; i < 8; i++ {
		packages[fmt.Sprintf("node_modules/demo%d", i)] = map[string]any{"version": "1.0.0"}
	}
	input := review5149JSON(t, map[string]any{"lockfileVersion": 3, "packages": packages})
	var baseline []byte
	for iteration := 0; iteration < 128; iteration++ {
		r, err := review5149Scan(map[string]string{"package-lock.json": input}, sca.WithResourceLimits(sca.ResourceLimits{MaxComponents: 1}))
		if r == nil || err == nil || r.Complete {
			t.Fatalf("expected a component-budget failure, report=%+v err=%v", r, err)
		}
		encoded := []byte(review5149JSON(t, r))
		if iteration == 0 {
			baseline = encoded
		} else if !bytes.Equal(baseline, encoded) {
			t.Fatalf("same input and budget selected different partial reports at iteration %d\nfirst=%s\nnow=%s", iteration, baseline, encoded)
		}
	}
}

// R03: the root lock record carries original declarations even without package.json.
func TestReview5149_NPMRootRequirements(t *testing.T) {
	input := `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0","dependencies":{"demo":"^1.0.0"}},"node_modules/demo":{"version":"1.2.0"}}}`
	r, err := review5149Scan(map[string]string{"package-lock.json": input})
	if err != nil || r == nil || !r.Complete {
		t.Fatalf("valid standalone lock failed: report=%+v err=%v", r, err)
	}
	for _, q := range r.Requirements {
		if q.Target == "demo" && q.Constraint == "^1.0.0" && len(q.Resolved) > 0 {
			return
		}
	}
	t.Fatalf("root declaration and resolved target were lost: %s", review5149JSON(t, r))
}

// R04: retain declared SRI evidence; this does not require downloading the tarball.
func TestReview5149_NPMIntegrityEvidence(t *testing.T) {
	sri := func(label string) string {
		sum := sha512.Sum512([]byte(label))
		return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
	}
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict_%v", conflict), func(t *testing.T) {
			packages := map[string]any{
				"":                  map[string]any{"name": "app", "version": "1.0.0", "dependencies": map[string]string{"demo": "1.2.0"}},
				"node_modules/demo": map[string]any{"name": "demo", "version": "1.2.0", "resolved": "https://example.invalid/demo.tgz", "integrity": sri("first")},
			}
			if conflict {
				packages["node_modules/wrapper"] = map[string]any{"name": "wrapper", "version": "1.0.0", "dependencies": map[string]string{"demo": "1.2.0"}}
				packages["node_modules/wrapper/node_modules/demo"] = map[string]any{"name": "demo", "version": "1.2.0", "resolved": "https://example.invalid/demo.tgz", "integrity": sri("second")}
			}
			input := review5149JSON(t, map[string]any{"lockfileVersion": 3, "packages": packages})
			r, err := review5149Scan(map[string]string{"package-lock.json": input})
			if r == nil {
				t.Fatalf("nil report: %v", err)
			}
			// Explicitly rejecting conflicting evidence is also a valid policy.
			if conflict && err != nil && !r.Complete {
				for _, d := range r.Diagnostics {
					text := strings.ToLower(d.Code + " " + d.Reason)
					if strings.Contains(text, "conflict") && (strings.Contains(text, "integrity") || strings.Contains(text, "digest") || strings.Contains(text, "checksum") || strings.Contains(text, "verification")) {
						return
					}
				}
			}
			if !conflict && (err != nil || !r.Complete) {
				t.Fatalf("valid integrity evidence rejected: err=%v report=%+v", err, r)
			}
			verifications := map[string]bool{}
			for _, c := range r.Components {
				if c.Key.Name == "demo" {
					if c.Key.Verification == "" {
						t.Fatalf("declared integrity silently dropped: %s", review5149JSON(t, r))
					}
					verifications[c.Key.Verification] = true
				}
			}
			want := 1
			if conflict {
				want = 2
			}
			if len(verifications) != want {
				t.Fatalf("digest evidence missing or conflated: got=%v want=%d err=%v", verifications, want, err)
			}
		})
	}
}

// R05: a frozen reader must reject unsupported versions and malformed identities.
func TestReview5149_SchemaAndIdentityValidation(t *testing.T) {
	cases := []struct{ name, file, text string }{
		{"pnpm_nan", "pnpm-lock.yaml", "lockfileVersion: 'NaN'\npackages:\n  /demo@1.0.0:\n    name: demo\n    version: 1.0.0\n"},
		{"pnpm_bad_identity", "pnpm-lock.yaml", "lockfileVersion: '6.0'\npackages:\n  '/demo@not-a-version':\n    dev: false\n"},
		{"cargo_future_version", "Cargo.lock", "version = 999\n[[package]]\nname = 'demo'\nversion = '1.0.0'\nsource = 'registry+https://example.invalid/index'\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := review5149Scan(map[string]string{tc.file: tc.text})
			if r == nil || err == nil || r.Complete {
				t.Fatalf("unsupported/malformed input reported successful: report=%+v err=%v", r, err)
			}
			for _, c := range r.Components {
				if c.Key.Name == "" {
					t.Fatal("malformed key produced an anonymous component")
				}
			}
		})
	}
}

// R06: unknown dependency version must not erase the declaring project edge.
func TestReview5149_POMUnknownVersionKeepsRequirement(t *testing.T) {
	input := `<project><modelVersion>4.0.0</modelVersion><groupId>org.review</groupId><artifactId>app</artifactId><version>1.0</version><dependencyManagement><dependencies><dependency><groupId>org.review</groupId><artifactId>missing-bom</artifactId><version>1.0</version><type>pom</type><scope>import</scope></dependency></dependencies></dependencyManagement><dependencies><dependency><groupId>org.review</groupId><artifactId>dep</artifactId></dependency></dependencies></project>`
	r, err := review5149Scan(map[string]string{"pom.xml": input})
	if r == nil {
		t.Fatalf("nil partial report: %v", err)
	}
	componentNames := map[string]string{}
	for _, c := range r.Components {
		componentNames[c.Key.ID()] = c.Key.Name
	}
	observationNames := map[string]string{}
	for _, o := range r.Observations {
		observationNames[o.ID()] = componentNames[o.Component]
	}
	for _, q := range r.Requirements {
		if observationNames[q.From] != "org.review:app" {
			continue
		}
		if q.Target == "org.review:dep" || strings.HasPrefix(q.Target, "org.review:dep:") {
			return
		}
		for _, id := range q.Resolved {
			if observationNames[id] == "org.review:dep" {
				return
			}
		}
	}
	t.Fatalf("missing version erased app -> dep declaration; err=%v report=%s", err, review5149JSON(t, r))
}

// R07: callers must be able to handle budget errors by a stable diagnostic code.
func TestReview5149_ReadLimitDiagnosticCode(t *testing.T) {
	input := `{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0"},"node_modules/demo":{"version":"1.2.0"}}}`
	r, err := review5149Scan(map[string]string{"package-lock.json": input}, sca.WithResourceLimits(sca.ResourceLimits{MaxFileBytes: 8}))
	if r == nil || err == nil || r.Complete {
		t.Fatalf("expected input-byte budget error: report=%+v err=%v", r, err)
	}
	for _, d := range r.Diagnostics {
		if d.Code == "resource_limit" {
			return
		}
	}
	t.Fatalf("budget error mislabeled as another failure: %s", review5149JSON(t, r))
}
