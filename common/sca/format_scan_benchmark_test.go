package sca

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/model"
)

// formatScanCase is one retained analyzer fixture for ScanReport timing.
// It is not a claim that these 20 paths are a complete old-vs-new matrix.
type formatScanCase struct {
	name, wantName, wantVersion string
	files                       map[string]string
	complete                    bool
	minComponents               int
}

func formatScanCases() []formatScanCase {
	return []formatScanCase{
		{name: "dpkg", files: map[string]string{"var/lib/dpkg/status": "testdata/dpkg/dpkg"}, complete: true, minComponents: 1, wantName: "adduser", wantVersion: "3.118ubuntu5"},
		{name: "rpm", files: map[string]string{"var/lib/rpm/rpmdb.sqlite": "testdata/rpm/rpmdb.sqlite"}, complete: true, minComponents: 1, wantName: "mariner-release", wantVersion: "2.0"},
		{name: "apk", files: map[string]string{"lib/apk/db/installed": "testdata/apk/apk"}, complete: true, minComponents: 1, wantName: "alpine-baselayout", wantVersion: "3.4.3-r1"},
		{name: "bundler", files: map[string]string{"Gemfile.lock": "testdata/ruby_bundler/positive/Gemfile.lock"}, complete: true, minComponents: 1, wantName: "metasploit-framework", wantVersion: "6.3.26"},
		{name: "cargo", files: map[string]string{"Cargo.lock": "testdata/rust_cargo/positive/Cargo.lock"}, complete: true, minComponents: 2, wantName: "aho-corasick", wantVersion: "0.7.20"},
		{name: "gemspec", files: map[string]string{"gems/specifications/test-unit.gemspec": "testdata/ruby_gemspec/positive/multiple_licenses.gemspec"}, complete: true, minComponents: 1, wantName: "test-unit", wantVersion: "3.3.7"},
		{name: "poetry", files: map[string]string{"poetry.lock": "testdata/python_poetry/positive/poetry.lock", "pyproject.toml": "testdata/python_poetry/positive/pyproject.toml"}, complete: true, minComponents: 1, wantName: "flask", wantVersion: "1.1.4"},
		{name: "pipenv", files: map[string]string{"Pipfile.lock": "testdata/python_pipenv/Pipfile.lock"}, complete: true, minComponents: 1, wantName: "pytz", wantVersion: "2022.7.1"},
		{name: "pip", files: map[string]string{"requirements.txt": "testdata/python_pip/requirements.txt"}, complete: true, minComponents: 1, wantName: "Flask", wantVersion: "2.0.0"},
		{name: "packaging", files: map[string]string{"x.dist-info/METADATA": "testdata/python_packaging/dist-info/METADATA"}, complete: true, minComponents: 1, wantName: "distlib", wantVersion: "0.3.1"},
		{name: "composer", files: map[string]string{"composer.lock": "testdata/php_composer/positive/composer.lock", "composer.json": "testdata/php_composer/positive/composer.json"}, complete: true, minComponents: 1, wantName: "pear/log", wantVersion: "1.13.3"},
		{name: "yarn", files: map[string]string{"yarn.lock": "testdata/node_yarn/positive/yarn.lock"}, complete: true, minComponents: 2, wantName: "js-tokens", wantVersion: "2.0.0"},
		{name: "pnpm", files: map[string]string{"pnpm-lock.yaml": "testdata/node_pnpm/pnpm-lock.yaml"}, complete: true, minComponents: 1, wantName: "lodash", wantVersion: "4.17.21"},
		{name: "npm", files: map[string]string{"package-lock.json": "testdata/node_npm/positive_folder/package-lock.json"}, complete: true, minComponents: 1, wantName: "ansi-colors", wantVersion: "3.2.3"},
		{name: "pom", files: map[string]string{"pom.xml": "testdata/java_pom/positive/pom.xml"}, complete: true, minComponents: 1, wantName: "com.example:example", wantVersion: "1.0.0"},
		{name: "gradle", files: map[string]string{"gradle.lockfile": "testdata/java_gradle/positive.lockfile"}, complete: true, minComponents: 1, wantName: "com.example:example", wantVersion: "0.0.1"},
		{name: "jar", files: map[string]string{"x.jar": "testdata/java_jar/positive/test.jar"}, complete: true, minComponents: 1, wantName: "org.apache:tomcat-embed-websocket", wantVersion: "9.0.65"},
		{name: "gomod", files: map[string]string{"go.mod": "testdata/go_mod/positive/mod", "go.sum": "testdata/go_mod/positive/sum"}, complete: true, minComponents: 1, wantName: "github.com/aquasecurity/go-dep-parser", wantVersion: "0.0.0-20220406074731-71021a481237"},
		{name: "gobinary", files: map[string]string{"app": "testdata/go_binary/go-binary"}, complete: true, minComponents: 1, wantName: "github.com/aquasecurity/go-pep440-version", wantVersion: "v0.0.0-20210121094942-22b2f8951d46"},
		{name: "conan", files: map[string]string{"conan.lock": "testdata/conan/conan"}, complete: true, minComponents: 1, wantName: "openssl", wantVersion: ""},
	}
}

func loadFormatScanFS(t testing.TB, files map[string]string) fstest.MapFS {
	t.Helper()
	in := fstest.MapFS{}
	for path, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		in[path] = &fstest.MapFile{Data: raw}
	}
	return in
}

func checkFormatScanContract(t testing.TB, r *model.Report, err error, tc formatScanCase) {
	t.Helper()
	if r == nil {
		t.Fatalf("%s: nil report: %v", tc.name, err)
	}
	if tc.complete {
		if err != nil || !r.Complete {
			t.Fatalf("%s: expected complete scan, complete=%v err=%v diag=%v", tc.name, r.Complete, err, r.Diagnostics)
		}
	} else if err == nil || r.Complete {
		t.Fatalf("%s: expected incomplete contract, complete=%v err=%v", tc.name, r.Complete, err)
	}
	if len(r.Components) < tc.minComponents {
		t.Fatalf("%s: components %d < %d", tc.name, len(r.Components), tc.minComponents)
	}
	if tc.wantName == "" {
		return
	}
	found := false
	for _, c := range r.Components {
		if c.Key.Name == tc.wantName || strings.HasPrefix(c.Key.Name, tc.wantName+"/") {
			if tc.wantVersion == "" || c.Key.Version == tc.wantVersion {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("%s: missing %s@%s in %+v", tc.name, tc.wantName, tc.wantVersion, r.Components)
	}
}

func TestFormatScanContract(t *testing.T) {
	cases := formatScanCases()
	if len(cases) != 20 {
		t.Fatalf("retained analyzer set is 20, got %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := loadFormatScanFS(t, tc.files)
			r, err := ScanReport(context.Background(), in, WithSnapshotID("format-scan-"+tc.name))
			checkFormatScanContract(t, r, err, tc)
		})
	}
}

// BenchmarkFormatScan times ScanReport on the 20 retained analyzers after the
// same semantic contract as TestFormatScanContract holds. Incomplete or empty
// inventories are failures, not faster results.
func BenchmarkFormatScan(b *testing.B) {
	for _, tc := range formatScanCases() {
		in := loadFormatScanFS(b, tc.files)
		r, err := ScanReport(context.Background(), in, WithSnapshotID("bench-"+tc.name))
		checkFormatScanContract(b, r, err, tc)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r, err := ScanReport(context.Background(), in, WithSnapshotID("bench-"+tc.name))
				checkFormatScanContract(b, r, err, tc)
			}
		})
	}
}
