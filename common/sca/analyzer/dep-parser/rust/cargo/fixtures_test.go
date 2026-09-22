// Fixed ZIP corpora are embedded exclusively in test compilation.
package cargo

import (
	"embed"

	"github.com/yaklang/yaklang/common/sca/internal/testcheck"
)

//go:embed testdata/cargo_lock_le_v3_corpus_v1.zip
var fixtureArchives embed.FS

var fixtures = testcheck.MustLoadFixtures(fixtureArchives)
