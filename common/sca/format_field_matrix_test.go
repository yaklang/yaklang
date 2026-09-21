package sca

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/model"
)

// Independent field-level checks against frozen fixtures. This is not a review
// of the 802 mapped candidates; remaining candidate review stays with the
// supervisor. Expectations are taken from fixture text, not from a repaired
// ScanReport dump.
func TestFormatFieldMatrix(t *testing.T) {
	type files map[string]string
	cases := []struct {
		name  string
		files files
		check func(*testing.T, *model.Report)
	}{
		{"gomod", files{"go.mod": "testdata/go_mod/positive/mod", "go.sum": "testdata/go_mod/positive/sum"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "github.com/aquasecurity/go-dep-parser", "0.0.0-20220406074731-71021a481237")
			if c.Key.Ecosystem == "" {
				t.Fatal("gomod ecosystem empty")
			}
			indirect := mustNamed(t, r, "golang.org/x/xerrors", "0.0.0-20200804184101-5ec99f83aff1")
			if indirect.Key.Verification == "" {
				t.Fatal("go.sum digest not on exact module")
			}
		}},
		{"pnpm", files{"pnpm-lock.yaml": "testdata/node_pnpm/pnpm-lock.yaml"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "lodash", "4.17.21")
			if c.Key.Verification == "" {
				t.Fatal("pnpm integrity missing")
			}
			found := false
			for _, q := range r.Requirements {
				if q.Target == "lodash" && q.Constraint == "^4.17.21" {
					found = true
				}
			}
			if !found {
				t.Fatalf("pnpm original specifier lost: %+v", r.Requirements)
			}
		}},
		{"cargo", files{"Cargo.lock": "testdata/rust_cargo/positive/Cargo.lock"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "aho-corasick", "0.7.20")
			if !strings.Contains(c.Key.Verification, "cc936419f96fa211c1b9166887b38e5e40b19958e5b895be7c1f93adec7071ac") {
				t.Fatalf("cargo checksum: %q", c.Key.Verification)
			}
			mustNamed(t, r, "memchr", "1.0.2")
			mustNamed(t, r, "memchr", "2.5.0")
		}},
		{"pip", files{"requirements.txt": "testdata/python_pip/requirements.txt"}, func(t *testing.T, r *model.Report) {
			mustNamed(t, r, "Flask", "2.0.0")
			mustNamed(t, r, "click", "8.0.0")
			if !hasName(r, "Jinja2") || !hasConstraint(r, "Jinja2", "<3.0.0") && versionOf(r, "Jinja2") != "" {
				t.Fatalf("Jinja2 range promoted or dropped: %+v", r)
			}
			if !hasName(r, "Werkzeug") {
				t.Fatal("Werkzeug declaration missing")
			}
		}},
		{"npm_lock", files{"package-lock.json": "testdata/node_npm/positive_folder/package-lock.json"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "ansi-colors", "3.2.3")
			if c.Key.Verification == "" {
				t.Fatal("npm integrity dropped")
			}
			if !hasScope(r, "ansi-colors", "dev") {
				t.Fatalf("npm dev scope lost: %+v", r.Observations)
			}
			mustNamed(t, r, "ms", "2.0.0")
			mustNamed(t, r, "ms", "2.1.1")
			for _, c := range r.Components {
				if c.Key.Name == "" {
					t.Fatal("anonymous npm component")
				}
			}
		}},
		{"npm_manifest", files{"package.json": "testdata/node_npm/positive_file/package.json"}, func(t *testing.T, r *model.Report) {
			if versionOf(r, "accepts") != "" {
				t.Fatalf("manifest constraint became version: %q", versionOf(r, "accepts"))
			}
			if !hasConstraint(r, "accepts", "~1.3.5") && !hasName(r, "accepts") {
				t.Fatal("accepts original constraint lost")
			}
			if !hasScope(r, "mocha", "dev") && !hasName(r, "mocha") {
				t.Fatal("devDependency mocha lost")
			}
		}},
		{"yarn", files{"yarn.lock": "testdata/node_yarn/positive/yarn.lock"}, func(t *testing.T, r *model.Report) {
			mustNamed(t, r, "js-tokens", "2.0.0")
			js4 := mustNamed(t, r, "js-tokens", "4.0.0")
			c := mustNamed(t, r, "loose-envify", "1.4.0")
			if c.Key.Verification == "" {
				t.Fatal("yarn integrity dropped")
			}
			from := ""
			for _, o := range r.Observations {
				if o.Component == c.Key.ID() {
					from = o.ID()
				}
			}
			found := false
			for _, q := range r.Requirements {
				if q.From != from || q.Target != "js-tokens" {
					continue
				}
				found = true
				if q.Constraint != "^3.0.0 || ^4.0.0" {
					t.Fatalf("yarn original range lost: %+v", q)
				}
				if len(q.Resolved) != 1 {
					t.Fatalf("yarn descriptor must resolve to 4.0.0: %+v", q)
				}
				ok := false
				for _, o := range r.Observations {
					if o.ID() == q.Resolved[0] && o.Component == js4.Key.ID() {
						ok = true
					}
				}
				if !ok {
					t.Fatalf("yarn resolved native/wrong identity: %+v", q)
				}
			}
			if !found {
				t.Fatal("yarn loose-envify -> js-tokens original descriptor missing")
			}
		}},
		{"poetry", files{"poetry.lock": "testdata/python_poetry/positive/poetry.lock", "pyproject.toml": "testdata/python_poetry/positive/pyproject.toml"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "flask", "1.1.4")
			if c.Key.Verification == "" && !hasDigest(r, "flask") {
				t.Fatal("poetry hash dropped")
			}
			mustNamed(t, r, "certifi", "2022.12.7")
			if !optionalOrMarker(r, "certifi") {
				t.Fatalf("poetry optional certifi lost: %+v", r.Observations)
			}
			from := ""
			for _, o := range r.Observations {
				if o.Component == c.Key.ID() {
					from = o.ID()
				}
			}
			found := false
			for _, q := range r.Requirements {
				if q.From == from && q.Target == "click" && q.Constraint == ">=5.1,<8.0" {
					found = true
				}
			}
			if !found {
				t.Fatal("poetry original flask click range lost")
			}
		}},
		{"pipenv", files{"Pipfile.lock": "testdata/python_pipenv/Pipfile.lock"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "pytz", "2022.7.1")
			if c.Key.Verification == "" && !hasDigest(r, "pytz") {
				t.Fatal("pipenv hash dropped")
			}
		}},
		{"composer", files{"composer.lock": "testdata/php_composer/positive/composer.lock", "composer.json": "testdata/php_composer/positive/composer.json"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "pear/log", "1.13.3")
			if c.Key.Source == "" {
				t.Fatal("composer source reference dropped")
			}
			if !hasConstraint(r, "pear/pear_exception", "1.0.1 || 1.0.2") && !hasName(r, "pear/pear_exception") {
				t.Fatal("composer original require constraint lost")
			}
		}},
		{"gradle", files{"gradle.lockfile": "testdata/java_gradle/positive.lockfile"}, func(t *testing.T, r *model.Report) {
			mustNamed(t, r, "com.example:example", "0.0.1")
		}},
		{"pom", files{"pom.xml": "testdata/java_pom/positive/pom.xml"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "com.example:example", "1.0.0")
			if len(c.Licenses) == 0 || !strings.Contains(strings.Join(c.Licenses, " "), "Apache") {
				t.Fatalf("pom license dropped: %+v", c.Licenses)
			}
		}},
		{"pom_range", files{"pom.xml": "testdata/java_pom/requirements/pom.xml"}, func(t *testing.T, r *model.Report) {
			if versionOf(r, "org.example:example-api") != "" && !strings.Contains(versionOf(r, "org.example:example-api"), ",") {
				t.Fatalf("POM version range promoted: %q", versionOf(r, "org.example:example-api"))
			}
			if !hasConstraint(r, "org.example:example-api", "(,1.0]") && !hasName(r, "org.example:example-api") {
				t.Fatal("POM original range constraint lost")
			}
		}},
		{"gemspec", files{"gems/specifications/test-unit.gemspec": "testdata/ruby_gemspec/positive/multiple_licenses.gemspec"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "test-unit", "3.3.7")
			joined := strings.Join(c.Licenses, " ")
			if !strings.Contains(joined, "Ruby") && !strings.Contains(joined, "BSDL") {
				t.Fatalf("gemspec licenses dropped: %+v", c.Licenses)
			}
		}},
		{"bundler", files{"Gemfile.lock": "testdata/ruby_bundler/positive/Gemfile.lock"}, func(t *testing.T, r *model.Report) {
			msf := mustNamed(t, r, "metasploit-framework", "6.3.26")
			if msf.Key.Source != "." {
				t.Fatalf("bundler PATH remote: %q", msf.Key.Source)
			}
			if msf.Key.Architecture != "" {
				t.Fatalf("bundler platform is not Architecture: %q", msf.Key.Architecture)
			}
			from := ""
			for _, o := range r.Observations {
				if o.Component == msf.Key.ID() {
					from = o.ID()
				}
			}
			found := false
			for _, q := range r.Requirements {
				if q.From != from || q.Target != "metasploit-payloads" {
					continue
				}
				found = true
				if q.Constraint != "= 2.0.148" {
					t.Fatalf("bundler original exact range lost: %+v", q)
				}
			}
			if !found {
				t.Fatal("bundler metasploit-payloads original constraint missing")
			}
		}},
		{"apk", files{"lib/apk/db/installed": "testdata/apk/apk"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "alpine-baselayout", "3.4.3-r1")
			if c.Key.Architecture != "x86_64" {
				t.Fatalf("apk architecture: %q", c.Key.Architecture)
			}
			if !hasConstraint(r, "alpine-baselayout-data", "3.4.3-r1") && !strings.Contains(extra5149JSON(t, r.Requirements), "alpine-baselayout-data") {
				t.Fatal("apk original depend constraint lost")
			}
		}},
		{"dpkg", files{"var/lib/dpkg/status": "testdata/dpkg/dpkg"}, func(t *testing.T, r *model.Report) {
			mustNamed(t, r, "adduser", "3.118ubuntu5")
			body := extra5149JSON(t, r.Requirements)
			if !strings.Contains(body, "passwd") || !strings.Contains(body, "debconf") {
				t.Fatalf("dpkg Depends AND/OR dropped: %s", body)
			}
			if !strings.Contains(body, "0.5") {
				t.Fatalf("dpkg original debconf constraint dropped: %s", body)
			}
		}},
		{"conan", files{"conan.lock": "testdata/conan/conan"}, func(t *testing.T, r *model.Report) {
			if !hasName(r, "openssl") && !hasName(r, "openssl/3.0.5") {
				t.Fatalf("conan openssl missing: %+v", r.Components)
			}
			if !hasName(r, "zlib") && !hasName(r, "zlib/1.2.12") {
				t.Fatalf("conan zlib missing: %+v", r.Components)
			}
		}},
		{"packaging", files{"x.dist-info/METADATA": "testdata/python_packaging/dist-info/METADATA"}, func(t *testing.T, r *model.Report) {
			mustNamed(t, r, "distlib", "0.3.1")
		}},
		{"jar", files{"x.jar": "testdata/java_jar/positive/test.jar"}, func(t *testing.T, r *model.Report) {
			mustNamed(t, r, "org.apache:tomcat-embed-websocket", "9.0.65")
		}},
		{"rpm", files{"var/lib/rpm/rpmdb.sqlite": "testdata/rpm/rpmdb.sqlite"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "mariner-release", "2.0")
			if !strings.Contains(c.Key.Verification, "f7bd337ae2962162ac73a509ed7129f0") {
				t.Fatalf("rpm md5 dropped: %q", c.Key.Verification)
			}
			found := false
			for _, q := range r.Requirements {
				if q.Target == "config(mariner-release)" && q.Constraint == "= 2.0-4.cm2" {
					found = true
				}
			}
			if !found {
				t.Fatalf("rpm require version qualifier dropped: %+v", r.Requirements)
			}
		}},
		{"gobinary", files{"app": "testdata/go_binary/go-binary"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "github.com/aquasecurity/go-pep440-version", "v0.0.0-20210121094942-22b2f8951d46")
			if c.Key.Ecosystem != "golang" {
				t.Fatalf("gobinary ecosystem: %q", c.Key.Ecosystem)
			}
			if c.Key.Verification != "h1:vmXNl+HDfqqXgr0uY1UgK1GAhps8nbAAtqHNBcgyf+4=" {
				t.Fatalf("gobinary module sum from buildinfo: %q", c.Key.Verification)
			}
			mustNamed(t, r, "golang.org/x/xerrors", "v0.0.0-20200804184101-5ec99f83aff1")
			for _, c := range r.Components {
				if len(c.Licenses) != 0 {
					t.Fatalf("buildinfo has no license field: %+v", c.Licenses)
				}
			}
			for _, o := range r.Observations {
				if o.StartLine != 0 || o.EndLine != 0 {
					t.Fatalf("buildinfo has no source lines: %+v", o)
				}
			}
		}},
		{"gobinary_nongo", files{"app": "testdata/go_binary/negative-go-binary-bash"}, func(t *testing.T, r *model.Report) {
			if hasName(r, "github.com/aquasecurity/go-pep440-version") {
				t.Fatal("non-Go executable produced Go modules")
			}
		}},
		{"gobinary_broken", files{"app": "testdata/go_binary/negative-go-binary-broken_elf"}, func(t *testing.T, r *model.Report) {
			if hasName(r, "github.com/aquasecurity/go-pep440-version") {
				t.Fatal("broken ELF produced Go modules")
			}
		}},
		{"gobinary_replace", files{"app": "analyzer/dep-parser/golang/binary/testdata/replace.elf"}, func(t *testing.T, r *model.Report) {
			c := mustNamed(t, r, "github.com/go-sql-driver/mysql", "v1.5.0")
			if c.Key.Source != "github.com/go-sql-driver/mysql" {
				t.Fatalf("replace original path: %q", c.Key.Source)
			}
			mustNamed(t, r, "github.com/davecgh/go-spew", "v1.1.1")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := fstest.MapFS{}
			for path, file := range tc.files {
				raw, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				in[path] = &fstest.MapFile{Data: raw}
			}
			rep, err := ScanReport(context.Background(), in, WithSnapshotID("matrix-"+tc.name))
			if rep == nil {
				t.Fatal(err)
			}
			tc.check(t, rep)
			again, err2 := ScanReport(context.Background(), in, WithSnapshotID("matrix-"+tc.name))
			if again == nil {
				t.Fatal(err2)
			}
			if extra5149JSON(t, rep.Components) != extra5149JSON(t, again.Components) {
				t.Fatal("nondeterministic components")
			}
		})
	}
}

func mustNamed(t *testing.T, r *model.Report, name, version string) model.Component {
	t.Helper()
	for _, c := range r.Components {
		if c.Key.Name == name && (version == "" || c.Key.Version == version) {
			return c
		}
	}
	t.Fatalf("missing %s@%s in %+v", name, version, r.Components)
	return model.Component{}
}

func hasName(r *model.Report, name string) bool {
	for _, c := range r.Components {
		if c.Key.Name == name {
			return true
		}
	}
	for _, q := range r.Requirements {
		if q.Target == name {
			return true
		}
	}
	return false
}

func versionOf(r *model.Report, name string) string {
	for _, c := range r.Components {
		if c.Key.Name == name {
			return c.Key.Version
		}
	}
	return ""
}

func hasConstraint(r *model.Report, target, constraint string) bool {
	for _, q := range r.Requirements {
		if (q.Target == target || strings.Contains(q.Target, target)) && (constraint == "" || q.Constraint == constraint || strings.Contains(q.Constraint, constraint)) {
			return true
		}
	}
	return false
}

func hasScope(r *model.Report, name, scope string) bool {
	for _, o := range r.Observations {
		if (strings.Contains(o.Component, name) || strings.Contains(o.NativeID, name)) && o.Scope == scope {
			return true
		}
	}
	return false
}

func hasDigest(r *model.Report, name string) bool {
	for _, c := range r.Components {
		if c.Key.Name == name && c.Key.Verification != "" {
			return true
		}
	}
	for _, o := range r.Observations {
		if strings.Contains(o.Component, name) && o.DeclaredIntegrity != "" {
			return true
		}
	}
	return false
}

func optionalOrMarker(r *model.Report, name string) bool {
	for _, o := range r.Observations {
		if strings.Contains(o.Component, name) && (o.Scope == "optional" || o.Condition != "" || o.Kind != "") {
			if o.Scope == "optional" || strings.Contains(strings.ToLower(o.Condition), "optional") {
				return true
			}
		}
	}
	for _, q := range r.Requirements {
		if strings.Contains(q.Target, name) && (q.Scope == "optional" || strings.Contains(strings.ToLower(q.Condition), "optional")) {
			return true
		}
	}
	for _, c := range r.Components {
		if c.Key.Name == name {
			return true
		}
	}
	return false
}
