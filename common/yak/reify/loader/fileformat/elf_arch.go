package fileformat

import (
	"debug/elf"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	elfimpl "github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/elf"
)

func elfArchitecture(machine elf.Machine, bits int) core.Architecture {
	return elfimpl.ELFArchitecture(machine, bits)
}
