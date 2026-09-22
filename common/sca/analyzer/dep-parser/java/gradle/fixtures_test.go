// Fixed ZIP corpora are embedded exclusively in test compilation.
package gradle

import (
	"embed"

	"github.com/yaklang/yaklang/common/sca/internal/testcheck"
)

//go:embed testdata/gradle_corpus_v1.zip
var fixtureArchives embed.FS

var fixtures = testcheck.MustLoadFixtures(fixtureArchives)
