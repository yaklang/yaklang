package fileformat

import (
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/elf"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/macho"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/pe"
)

// All returns one loader per supported native file format.
func All() []core.Loader {
	return []core.Loader{elf.Loader{}, pe.Loader{}, macho.Loader{}}
}
