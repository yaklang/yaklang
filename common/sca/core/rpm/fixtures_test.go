// Fixed ZIP corpora are embedded exclusively in test compilation.
package rpm

import (
	"embed"

	"github.com/yaklang/yaklang/common/sca/internal/testcheck"
)

//go:embed testdata/rpm_bdb_ndb_sqlite_corpus_v1.zip
//go:embed testdata/rpm_libuuid_header_corpus_v1.zip
var fixtureArchives embed.FS

var fixtures = testcheck.MustLoadFixtures(fixtureArchives)
