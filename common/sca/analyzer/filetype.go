package analyzer

import "github.com/h2non/filetype"

var fastELFType = filetype.NewType("fast-elf", "application/x-executable")

func fastELFTypeMatcher(buf []byte) bool {
	return len(buf) > 3 && buf[0] == 0x7F && buf[1] == 0x45 &&
		buf[2] == 0x4C && buf[3] == 0x46
}

func init() {
	filetype.AddMatcher(fastELFType, fastELFTypeMatcher)
}

func IsExecutable(buf []byte) bool {
	return filetype.Is(buf, "exe") || filetype.Is(buf, "fast-elf")
}
