package stream_parser

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

// DecodeSMB2DirectoryEntries decodes FILE_*_DIRECTORY_INFORMATION chains
// without requiring the matched request class. The class only selects the
// class-specific tail (EaSize/ShortName/FileId); the common prefix, entry
// terminator and UTF-16 file name are class-independent for every layout
// except FILE_NAMES_INFORMATION (class 12), which has its own small shape.
//
// class 0 means "auto": the name offset is selected from the documented
// class-specific offsets and validated against the entry boundary. A declared
// class (1,2,3,4,12,37) is parsed strictly.
func DecodeSMB2DirectoryEntries(wire []byte, class uint8, max int) (map[string]any, error) {
	entries, status, err := decodeSMB2DirectoryEntries(wire, class, max)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"Status":  status,
		"Entries": entries,
	}, nil
}

func decodeSMB2DirectoryEntries(wire []byte, class uint8, max int) ([]map[string]any, string, error) {
	if len(wire) == 0 {
		return nil, "", fmt.Errorf("directory output is empty")
	}
	if len(wire) > 1<<20 {
		return nil, "", fmt.Errorf("directory output exceeds 1 MiB")
	}
	if max <= 0 {
		max = 1
	}
	switch class {
	case 0:
		entries, err := decodeSMB2FileDirectoryEntries(wire, 0, max)
		if err == nil {
			return entries, "generic FILE_*_DIRECTORY_INFORMATION prefix; class-specific tail not selected", nil
		}
		// FILE_NAMES_INFORMATION is the one selected class with a different
		// prefix. Try it when the common-prefix chain does not validate.
		if names, namesErr := decodeSMB2FileNamesEntries(wire, max); namesErr == nil {
			return names, "FileNamesInformation (auto-selected)", nil
		}
		return nil, "", err
	case 12:
		entries, err := decodeSMB2FileNamesEntries(wire, max)
		if err != nil {
			return nil, "", err
		}
		return entries, "FileNamesInformation", nil
	case 1, 2, 3, 37, 38:
		entries, err := decodeSMB2FileDirectoryEntries(wire, class, max)
		if err != nil {
			return nil, "", err
		}
		return entries, smb2DirectoryClassName(class), nil
	default:
		return nil, "", fmt.Errorf("directory information class %d is not a selected layout", class)
	}
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
	return ""
}

func smb2DirectoryNameOffsets(class uint8) []int {
	switch class {
	case 1:
		return []int{64}
	case 2:
		return []int{68}
	case 3:
		return []int{94}
	case 37:
		return []int{104}
	case 38:
		return []int{76}
	}
	// Auto: documented class-specific name offsets. The first candidate whose
	// name ends at the entry boundary is the class-independent proof.
	return []int{64, 68, 76, 94, 104}
}

// smb2DirectoryName selects the unique class-specific name offset that makes
// the declared UTF-16 name end at the entry boundary (at most seven zero pad
// bytes). An offset inside the fixed tail cannot satisfy that boundary.
func smb2DirectoryName(wire []byte, start, end, nameLen int, class uint8) (string, int, bool) {
	if nameLen <= 0 || nameLen%2 != 0 {
		return "", 0, false
	}
	for _, offset := range smb2DirectoryNameOffsets(class) {
		nameStart := start + offset
		nameEnd := nameStart + nameLen
		if nameStart < start || nameEnd > end || end-nameEnd >= 8 {
			continue
		}
		if !smb2ZeroPad(wire[nameEnd:end]) {
			continue
		}
		text, err := DecodeCIFSUTF16(wire[nameStart:nameEnd])
		if err != nil || strings.IndexByte(text, 0) >= 0 {
			continue
		}
		return text, offset, true
	}
	return "", 0, false
}

func smb2ZeroPad(padding []byte) bool {
	for _, b := range padding {
		if b != 0 {
			return false
		}
	}
	return true
}

func decodeSMB2FileDirectoryEntries(wire []byte, class uint8, max int) ([]map[string]any, error) {
	var entries []map[string]any
	for at := 0; ; {
		if len(entries) >= max {
			return nil, fmt.Errorf("directory entry count exceeds collection budget")
		}
		if len(wire)-at < 64 {
			return nil, fmt.Errorf("truncated directory entry at offset %d", at)
		}
		next := int(binary.LittleEndian.Uint32(wire[at : at+4]))
		end := len(wire)
		if next != 0 {
			if next < 64 || next%8 != 0 || next > len(wire)-at {
				return nil, fmt.Errorf("directory NextEntryOffset leaves the output buffer")
			}
			end = at + next
		}
		nameLen := int(binary.LittleEndian.Uint32(wire[at+60 : at+64]))
		text, nameOffset, ok := smb2DirectoryName(wire, at, end, nameLen, class)
		if !ok {
			return nil, fmt.Errorf("directory file name does not end this entry")
		}
		endOfFile := int64(binary.LittleEndian.Uint64(wire[at+40 : at+48]))
		allocation := int64(binary.LittleEndian.Uint64(wire[at+48 : at+56]))
		if endOfFile < 0 || allocation < 0 {
			return nil, fmt.Errorf("directory size is a negative LARGE_INTEGER")
		}
		entry := map[string]any{
			"Next Entry Offset": uint32(next),
			"File Index":        binary.LittleEndian.Uint32(wire[at+4 : at+8]),
			"Creation Time":     binary.LittleEndian.Uint64(wire[at+8 : at+16]),
			"Last Access Time":  binary.LittleEndian.Uint64(wire[at+16 : at+24]),
			"Last Write Time":   binary.LittleEndian.Uint64(wire[at+24 : at+32]),
			"Change Time":       binary.LittleEndian.Uint64(wire[at+32 : at+40]),
			"End Of File":       endOfFile,
			"Allocation Size":   allocation,
			"File Attributes":   binary.LittleEndian.Uint32(wire[at+56 : at+60]),
			"File Name Length":  uint32(nameLen),
			"File Name":         text,
			"Wire Field Ranges": map[string]any{
				"Next Entry Offset": []any{uint64(at), uint64(at + 4)},
				"File Index":        []any{uint64(at + 4), uint64(at + 8)},
				"End Of File":       []any{uint64(at + 40), uint64(at + 48)},
				"Allocation Size":   []any{uint64(at + 48), uint64(at + 56)},
				"File Attributes":   []any{uint64(at + 56), uint64(at + 60)},
				"File Name Length":  []any{uint64(at + 60), uint64(at + 64)},
				"File Name":         []any{uint64(at + nameOffset), uint64(at + nameOffset + nameLen)},
			},
		}
		for index, key := range []string{"Creation Time", "Last Access Time", "Last Write Time", "Change Time"} {
			from := at + 8 + 8*index
			entry["Wire Field Ranges"].(map[string]any)[key] = []any{uint64(from), uint64(from + 8)}
		}
		switch class {
		case 2, 3, 37, 38:
			entry["EA Size"] = binary.LittleEndian.Uint32(wire[at+64 : at+68])
			entry["Wire Field Ranges"].(map[string]any)["EA Size"] = []any{uint64(at + 64), uint64(at + 68)}
		}
		switch class {
		case 3, 37:
			shortLen := int(wire[at+68])
			if shortLen > 24 || shortLen%2 != 0 {
				return nil, fmt.Errorf("directory short name length is not a bounded UTF-16 span")
			}
			entry["Short Name Length"] = uint8(shortLen)
			entry["Wire Field Ranges"].(map[string]any)["Short Name Length"] = []any{uint64(at + 68), uint64(at + 69)}
			if shortLen > 0 {
				shortName, err := DecodeCIFSUTF16(wire[at+70 : at+70+shortLen])
				if err != nil || strings.IndexByte(shortName, 0) >= 0 {
					return nil, fmt.Errorf("directory short name is not a valid UTF-16 name")
				}
				entry["Short Name"] = shortName
				entry["Wire Field Ranges"].(map[string]any)["Short Name"] = []any{uint64(at + 70), uint64(at + 70 + shortLen)}
			}
		}
		switch class {
		case 38:
			entry["File ID"] = hex.EncodeToString(wire[at+68 : at+76])
			entry["Wire Field Ranges"].(map[string]any)["File ID"] = []any{uint64(at + 68), uint64(at + 76)}
		case 37:
			entry["File ID"] = hex.EncodeToString(wire[at+96 : at+104])
			entry["Wire Field Ranges"].(map[string]any)["File ID"] = []any{uint64(at + 96), uint64(at + 104)}
		}
		entries = append(entries, entry)
		if next == 0 {
			break
		}
		at = end
	}
	return entries, nil
}

func decodeSMB2FileNamesEntries(wire []byte, max int) ([]map[string]any, error) {
	var entries []map[string]any
	for at := 0; ; {
		if len(entries) >= max {
			return nil, fmt.Errorf("directory entry count exceeds collection budget")
		}
		if len(wire)-at < 12 {
			return nil, fmt.Errorf("truncated FileNamesInformation entry at offset %d", at)
		}
		next := int(binary.LittleEndian.Uint32(wire[at : at+4]))
		end := len(wire)
		if next != 0 {
			if next < 12 || next%8 != 0 || next > len(wire)-at {
				return nil, fmt.Errorf("directory NextEntryOffset leaves the output buffer")
			}
			end = at + next
		}
		nameLen := int(binary.LittleEndian.Uint32(wire[at+8 : at+12]))
		if nameLen <= 0 || nameLen%2 != 0 || at+12+nameLen > end || end-(at+12+nameLen) >= 8 {
			return nil, fmt.Errorf("FileNamesInformation name does not end this entry")
		}
		if !smb2ZeroPad(wire[at+12+nameLen : end]) {
			return nil, fmt.Errorf("FileNamesInformation entry has non-zero trailing bytes")
		}
		text, err := DecodeCIFSUTF16(wire[at+12 : at+12+nameLen])
		if err != nil || strings.IndexByte(text, 0) >= 0 {
			return nil, fmt.Errorf("FileNamesInformation name is not a valid UTF-16 name")
		}
		entries = append(entries, map[string]any{
			"Next Entry Offset": uint32(next),
			"File Index":        binary.LittleEndian.Uint32(wire[at+4 : at+8]),
			"File Name Length":  uint32(nameLen),
			"File Name":         text,
			"Wire Field Ranges": map[string]any{
				"Next Entry Offset": []any{uint64(at), uint64(at + 4)},
				"File Index":        []any{uint64(at + 4), uint64(at + 8)},
				"File Name Length":  []any{uint64(at + 8), uint64(at + 12)},
				"File Name":         []any{uint64(at + 12), uint64(at + 12 + nameLen)},
			},
		})
		if next == 0 {
			break
		}
		at = end
	}
	return entries, nil
}
