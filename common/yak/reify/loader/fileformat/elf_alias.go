package fileformat

import elfimpl "github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/elf"

// ELF keeps the native-format name available while the implementation lives in
// the dedicated ELF subpackage.
type ELF = elfimpl.Loader
