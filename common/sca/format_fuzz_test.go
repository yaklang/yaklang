package sca

import (
	"context"
	"testing"
	"testing/fstest"
)

// Every frozen text/ZIP entry point is reached using its logical discovery path.
// RPM's complete BDB/NDB/SQLite and Header seeds live in core/rpm.
func FuzzFrozenFormats(f *testing.F) {
	seeds := map[string]string{
		"var/lib/dpkg/status": "testdata/dpkg/dpkg", "lib/apk/db/installed": "testdata/apk/apk", "Gemfile.lock": "testdata/ruby_bundler/positive/Gemfile.lock", "Cargo.lock": "testdata/rust_cargo/positive/Cargo.lock", "a.gemspec": "testdata/ruby_gemspec/positive/multiple_licenses.gemspec", "poetry.lock": "testdata/python_poetry/positive/poetry.lock", "Pipfile.lock": "testdata/python_pipenv/Pipfile.lock", "requirements.txt": "testdata/python_pip/requirements.txt", "x.dist-info/METADATA": "testdata/python_packaging/dist-info/METADATA", "composer.lock": "testdata/php_composer/positive/composer.lock", "composer.json": "testdata/php_composer/positive/composer.json", "yarn.lock": "testdata/node_yarn/positive/yarn.lock", "pnpm-lock.yaml": "testdata/node_pnpm/pnpm-lock.yaml", "package-lock.json": "testdata/node_npm/positive_folder/package-lock.json", "package.json": "testdata/node_npm/positive_file/package.json", "pom.xml": "testdata/java_pom/positive/pom.xml", "gradle.lockfile": "testdata/java_gradle/positive.lockfile", "x.jar": "testdata/java_jar/positive/test.jar", "go.mod": "testdata/go_mod/positive/mod", "conan.lock": "testdata/conan/conan"}
	for name, file := range seeds {
		b, e := fixtures.ReadFile(file)
		if e != nil {
			f.Fatal(e)
		}
		if len(b) > 4<<20 {
			f.Fatalf("oversize fuzz seed %s", file)
		}
		f.Add(name, b)
		f.Add(name, b[:len(b)/2])
		f.Add(name, []byte("\x00\xff{{[[\""))
	}
	f.Fuzz(func(t *testing.T, name string, b []byte) {
		if _, ok := seeds[name]; !ok || len(b) > 4<<20 {
			return
		}
		r, _ := ScanReport(context.Background(), fstest.MapFS{name: {Data: b}}, WithResourceLimits(ResourceLimits{MaxFileBytes: 4 << 20, MaxFiles: 32, MaxCandidateFiles: 32, MaxTotalReadBytes: 32 << 20, MaxExpandedBytes: 8 << 20, MaxArchiveEntries: 1000, MaxComponents: 10000, MaxObservations: 20000, MaxEdges: 50000, MaxExpressionNodes: 50000}))
		for _, d := range r.Diagnostics {
			if d.Code == "internal_error" {
				t.Fatal(d)
			}
		}
	})
}
