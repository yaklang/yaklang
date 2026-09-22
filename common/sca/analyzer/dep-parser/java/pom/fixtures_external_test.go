// Fixed ZIP corpora are embedded exclusively in test compilation.
package pom_test

import (
	"embed"

	"github.com/yaklang/yaklang/common/sca/internal/testcheck"
)

//go:embed testdata/maven_pom_corpus_v1.zip
var fixtureArchives embed.FS

var fixtures = testcheck.MustLoadFixtures(fixtureArchives)
