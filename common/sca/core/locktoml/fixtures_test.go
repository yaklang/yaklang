// Fixed ZIP corpora are embedded exclusively in test compilation.
package locktoml

import (
	"embed"

	"github.com/yaklang/yaklang/common/sca/internal/testcheck"
)

//go:embed testdata/toml_v1_0_and_rejected_v1_1_corpus_v1.zip
var fixtureArchives embed.FS

var fixtures = testcheck.MustLoadFixtures(fixtureArchives)
