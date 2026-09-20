package sca

import (
	"context"
	"os"
	"testing"
	"testing/fstest"
)

// BenchmarkFormatScan measures ScanReport on frozen fixtures. It is the
// format-level companion to BenchmarkExactIdentity (merge-only) and
// BenchmarkSparseDiscovery (open/read counts). Logs belong under the
// supervision directory or /tmp; they are not repository artifacts.
func BenchmarkFormatScan(b *testing.B) {
	cases := []struct{ name, path, file string }{
		{"gomod", "go.mod", "testdata/go_mod/positive/mod"},
		{"npm", "package-lock.json", "testdata/node_npm/positive_folder/package-lock.json"},
		{"pnpm", "pnpm-lock.yaml", "testdata/node_pnpm/pnpm-lock.yaml"},
		{"yarn", "yarn.lock", "testdata/node_yarn/positive/yarn.lock"},
		{"cargo", "Cargo.lock", "testdata/rust_cargo/positive/Cargo.lock"},
		{"pip", "requirements.txt", "testdata/python_pip/requirements.txt"},
		{"pipenv", "Pipfile.lock", "testdata/python_pipenv/Pipfile.lock"},
		{"poetry", "poetry.lock", "testdata/python_poetry/positive/poetry.lock"},
		{"composer", "composer.lock", "testdata/php_composer/positive/composer.lock"},
		{"gradle", "gradle.lockfile", "testdata/java_gradle/positive.lockfile"},
		{"pom", "pom.xml", "testdata/java_pom/positive/pom.xml"},
		{"bundler", "Gemfile.lock", "testdata/ruby_bundler/positive/Gemfile.lock"},
		{"apk", "lib/apk/db/installed", "testdata/apk/apk"},
		{"dpkg", "var/lib/dpkg/status", "testdata/dpkg/dpkg"},
		{"conan", "conan.lock", "testdata/conan/conan"},
		{"packaging", "x.dist-info/METADATA", "testdata/python_packaging/dist-info/METADATA"},
		{"jar", "x.jar", "testdata/java_jar/positive/test.jar"},
		{"rpm", "var/lib/rpm/rpmdb.sqlite", "testdata/rpm/rpmdb.sqlite"},
	}
	for _, tc := range cases {
		raw, err := os.ReadFile(tc.file)
		if err != nil {
			b.Fatal(err)
		}
		input := fstest.MapFS{tc.path: {Data: raw}}
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r, err := ScanReport(context.Background(), input, WithSnapshotID("bench-"+tc.name))
				if r == nil {
					b.Fatal(err)
				}
			}
		})
	}
}
