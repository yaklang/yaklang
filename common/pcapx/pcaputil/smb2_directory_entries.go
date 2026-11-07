package pcaputil

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// smb2DecodeDirectoryEntries adapts the shared, class-aware directory chain
// decoder to native events. Entry ranges are output-buffer-relative in the
// shared result and are rebased to the owned PDU here.
func smb2DecodeDirectoryEntries(raw []byte, class uint8, max, base int) ([]map[string]any, string, error) {
	decoded, err := stream_parser.DecodeSMB2DirectoryEntries(raw, class, max)
	if err != nil {
		return nil, "", err
	}
	entries, _ := decoded["Entries"].([]map[string]any)
	status, _ := decoded["Status"].(string)
	if base > 0 {
		for _, entry := range entries {
			smb2OffsetRanges(entry, base)
		}
	}
	return entries, status, nil
}

func smb2DirectoryClassName(class uint8) string {
	switch class {
	case 1:
		return "FileDirectoryInformation"
	case 2:
		return "FileFullDirectoryInformation"
	case 3:
		return "FileBothDirectoryInformation"
	case 12:
		return "FileNamesInformation"
	case 37:
		return "FileIdBothDirectoryInformation"
	case 38:
		return "FileIdFullDirectoryInformation"
	}
	return "directory information"
}
