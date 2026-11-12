package fileformat

import (
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/coff"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/elf"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/macho"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/mz"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/pe"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/unixar"
)

// All returns one loader per supported native file format. The order is
// significant: a container that wraps other formats comes after the formats it
// can contain, and a legacy header comes last so its shorter magic cannot claim
// a file another loader validates.
func All() []core.Loader {
	return []core.Loader{
		elf.Loader{}, pe.Loader{}, macho.Loader{}, coff.Loader{},
		unixar.Loader{}, mz.Loader{},
	}
}

// init wires the archive member table. An archive resolves each member with the
// loader that matches its header, so it needs the set after all loaders exist.
// The table excludes the archive loader itself, which cannot nest.
func init() {
	members := make([]core.Loader, 0, len(All()))
	for _, loader := range All() {
		if loader.Kind() == core.FormatArchive {
			continue
		}
		members = append(members, loader)
	}
	unixar.SetMemberLoaders(members)
}
