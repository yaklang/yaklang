package fileformat

import (
	"github.com/yaklang/yaklang/common/yak/reify/core"
	elfimpl "github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/elf"
)

// All returns one loader per supported native file format.
func All() []core.Loader {
	return []core.Loader{elfimpl.Loader{}, PE{}, MachO{}}
}
