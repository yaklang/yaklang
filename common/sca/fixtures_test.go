// Fixed ZIP corpora are embedded exclusively in test compilation.
package sca

import (
	"embed"

	"github.com/yaklang/yaklang/common/sca/internal/testcheck"
)

//go:embed analyzer/dep-parser/c/conan/testdata/conan_corpus_v1.zip
//go:embed analyzer/dep-parser/golang/binary/testdata/go_buildinfo_elf_pe_macho_corpus_v1.zip
//go:embed analyzer/dep-parser/java/gradle/testdata/gradle_corpus_v1.zip
//go:embed analyzer/dep-parser/java/pom/testdata/maven_pom_corpus_v1.zip
//go:embed analyzer/dep-parser/python/packaging/testdata/packaging_corpus_v1.zip
//go:embed analyzer/testdata/jar_nested_archives_corpus_v1.zip
//go:embed core/rpm/testdata/rpm_libuuid_header_corpus_v1.zip
//go:embed testdata/apk_corpus_v1.zip
//go:embed testdata/cargo_lock_le_v3_corpus_v1.zip
//go:embed testdata/conan_corpus_v1.zip
//go:embed testdata/dpkg_corpus_v1.zip
//go:embed testdata/full_field_oracles_corpus_v1.zip
//go:embed testdata/go_buildinfo_elf_pe_corpus_v1.zip
//go:embed testdata/go_mod_corpus_v1.zip
//go:embed testdata/java_gradle_corpus_v1.zip
//go:embed testdata/java_jar_corpus_v1.zip
//go:embed testdata/maven_pom_corpus_v1.zip
//go:embed testdata/npm_lock_corpus_v1.zip
//go:embed testdata/php_composer_corpus_v1.zip
//go:embed testdata/pnpm_lock_v5_v6_corpus_v1.zip
//go:embed testdata/python_packaging_corpus_v1.zip
//go:embed testdata/python_pip_corpus_v1.zip
//go:embed testdata/python_pipenv_corpus_v1.zip
//go:embed testdata/python_poetry_corpus_v1.zip
//go:embed testdata/rpm_bdb_ndb_sqlite_corpus_v1.zip
//go:embed testdata/ruby_bundler_corpus_v1.zip
//go:embed testdata/ruby_gemspec_corpus_v1.zip
//go:embed testdata/yarn_classic_berry_corpus_v1.zip
var fixtureArchives embed.FS

var fixtures = testcheck.MustLoadFixtures(fixtureArchives)
