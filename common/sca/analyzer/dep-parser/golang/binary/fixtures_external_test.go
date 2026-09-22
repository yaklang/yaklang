// Fixed ZIP corpora are embedded exclusively in test compilation.
package binary_test

import (
	"embed"

	"github.com/yaklang/yaklang/common/sca/internal/testcheck"
)

//go:embed testdata/go_buildinfo_elf_pe_macho_corpus_v1.zip
var fixtureArchives embed.FS

var fixtures = testcheck.MustLoadFixtures(fixtureArchives)
