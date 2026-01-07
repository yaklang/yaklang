package yaklib

import (
	"debug/elf"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// bytesReaderAt 实现 io.ReaderAt 接口
type bytesReaderAt struct {
	data []byte
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, fmt.Errorf("negative offset")
	}
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n = copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// ELFHeader 表示ELF文件头信息
type ELFHeader struct {
	Magic      string `json:"magic"`      // ELF魔数（如 "ELF"）
	Class      string `json:"class"`      // 架构类型：32位或64位
	Data       string `json:"data"`       // 字节序：little-endian或big-endian
	Version    uint32 `json:"version"`    // ELF版本
	OSABI      uint8  `json:"osabi"`      // 操作系统ABI
	ABIVersion uint8  `json:"abiversion"` // ABI版本
	Type       string `json:"type"`       // 文件类型：ET_REL, ET_EXEC, ET_DYN, ET_CORE
	Machine    string `json:"machine"`    // 机器架构：EM_X86_64, EM_386, EM_ARM等
	Entry      uint64 `json:"entry"`      // 入口地址
}

// ELFSegment 表示ELF段信息
type ELFSegment struct {
	Type   string `json:"type"`    // 段类型：PT_LOAD, PT_DYNAMIC等
	Flags  string `json:"flags"`   // 段标志：R, W, X的组合
	Offset uint64 `json:"offset"`  // 段在文件中的偏移
	VAddr  uint64 `json:"vaddr"`   // 虚拟地址
	PAddr  uint64 `json:"paddr"`   // 物理地址
	FileSz uint64 `json:"filesz"`  // 段在文件中的大小
	MemSz  uint64 `json:"memsz"`   // 段在内存中的大小
	Align  uint64 `json:"align"`   // 对齐方式
	IsCode bool   `json:"is_code"` // 是否为代码段
	IsData bool   `json:"is_data"` // 是否为数据段
}

// ELFSection 表示ELF节信息
type ELFSection struct {
	Name     string `json:"name"`      // 节名称
	Type     string `json:"type"`      // 节类型：SHT_PROGBITS, SHT_SYMTAB等
	Flags    string `json:"flags"`     // 节标志：SHF_WRITE, SHF_ALLOC等
	Addr     uint64 `json:"addr"`      // 节地址
	Offset   uint64 `json:"offset"`    // 节在文件中的偏移
	Size     uint64 `json:"size"`      // 节大小
	Link     uint32 `json:"link"`      // 链接信息
	Info     uint32 `json:"info"`      // 附加信息
	Align    uint64 `json:"align"`     // 对齐方式
	EntSize  uint64 `json:"entsize"`   // 条目大小
	IsSymTab bool   `json:"is_symtab"` // 是否为符号表
	IsStrTab bool   `json:"is_strtab"` // 是否为字符串表
}

// ELFInfo 包含完整的ELF文件信息
type ELFInfo struct {
	Header   *ELFHeader   `json:"header"`
	Segments []ELFSegment `json:"segments"`
	Sections []ELFSection `json:"sections"`
}

// parseELFHeader 解析ELF文件头
func parseELFHeader(f *elf.File) *ELFHeader {
	header := &ELFHeader{
		Magic:      "ELF",
		Version:    uint32(f.FileHeader.Version),
		OSABI:      uint8(f.FileHeader.OSABI),
		ABIVersion: f.FileHeader.ABIVersion,
		Entry:      f.FileHeader.Entry,
	}

	// 解析Class（32位或64位）
	switch f.FileHeader.Class {
	case elf.ELFCLASS32:
		header.Class = "32-bit"
	case elf.ELFCLASS64:
		header.Class = "64-bit"
	default:
		header.Class = "unknown"
	}

	// 解析Data（字节序）
	switch f.FileHeader.Data {
	case elf.ELFDATA2LSB:
		header.Data = "little-endian"
	case elf.ELFDATA2MSB:
		header.Data = "big-endian"
	default:
		header.Data = "unknown"
	}

	// 解析Type（文件类型）
	switch f.FileHeader.Type {
	case elf.ET_REL:
		header.Type = "ET_REL (Relocatable file)"
	case elf.ET_EXEC:
		header.Type = "ET_EXEC (Executable file)"
	case elf.ET_DYN:
		header.Type = "ET_DYN (Shared object file)"
	case elf.ET_CORE:
		header.Type = "ET_CORE (Core file)"
	default:
		header.Type = fmt.Sprintf("Unknown (0x%x)", f.FileHeader.Type)
	}

	// 解析Machine（机器架构）
	machineMap := map[elf.Machine]string{
		elf.EM_386:         "EM_386 (Intel 80386)",
		elf.EM_X86_64:      "EM_X86_64 (AMD x86-64)",
		elf.EM_ARM:         "EM_ARM (ARM)",
		elf.EM_AARCH64:     "EM_AARCH64 (ARM 64-bit)",
		elf.EM_MIPS:        "EM_MIPS (MIPS)",
		elf.EM_PPC:         "EM_PPC (PowerPC)",
		elf.EM_PPC64:       "EM_PPC64 (PowerPC 64-bit)",
		elf.EM_RISCV:       "EM_RISCV (RISC-V)",
		elf.EM_SPARC:       "EM_SPARC (SPARC)",
		elf.EM_SPARC32PLUS: "EM_SPARC32PLUS (SPARC v8+)",
		elf.EM_SPARCV9:     "EM_SPARCV9 (SPARC v9)",
		elf.EM_IA_64:       "EM_IA_64 (Intel IA-64)",
		elf.EM_S390:        "EM_S390 (IBM S/390)",
		elf.EM_SH:          "EM_SH (SuperH)",
		elf.EM_ALPHA:       "EM_ALPHA (Alpha)",
	}
	if name, ok := machineMap[f.FileHeader.Machine]; ok {
		header.Machine = name
	} else {
		header.Machine = fmt.Sprintf("Unknown (0x%x)", f.FileHeader.Machine)
	}

	return header
}

// parseELFSegments 解析ELF段
func parseELFSegments(f *elf.File) []ELFSegment {
	segments := make([]ELFSegment, 0)
	progs := f.Progs
	if progs == nil {
		return segments
	}

	for _, prog := range progs {
		seg := ELFSegment{
			Offset: uint64(prog.Off),
			VAddr:  uint64(prog.Vaddr),
			PAddr:  uint64(prog.Paddr),
			FileSz: uint64(prog.Filesz),
			MemSz:  uint64(prog.Memsz),
			Align:  uint64(prog.Align),
		}

		// 解析段类型
		segmentTypeMap := map[elf.ProgType]string{
			elf.PT_NULL:         "PT_NULL",
			elf.PT_LOAD:         "PT_LOAD",
			elf.PT_DYNAMIC:      "PT_DYNAMIC",
			elf.PT_INTERP:       "PT_INTERP",
			elf.PT_NOTE:         "PT_NOTE",
			elf.PT_SHLIB:        "PT_SHLIB",
			elf.PT_PHDR:         "PT_PHDR",
			elf.PT_TLS:          "PT_TLS",
			elf.PT_GNU_EH_FRAME: "PT_GNU_EH_FRAME",
			elf.PT_GNU_STACK:    "PT_GNU_STACK",
			elf.PT_GNU_RELRO:    "PT_GNU_RELRO",
		}
		if name, ok := segmentTypeMap[prog.Type]; ok {
			seg.Type = name
		} else {
			seg.Type = fmt.Sprintf("Unknown (0x%x)", prog.Type)
		}

		// 解析段标志
		flags := ""
		if prog.Flags&elf.PF_R != 0 {
			flags += "R"
		}
		if prog.Flags&elf.PF_W != 0 {
			flags += "W"
		}
		if prog.Flags&elf.PF_X != 0 {
			flags += "X"
		}
		if flags == "" {
			flags = "-"
		}
		seg.Flags = flags

		// 判断是否为代码段或数据段
		if prog.Type == elf.PT_LOAD {
			if prog.Flags&elf.PF_X != 0 {
				seg.IsCode = true
			}
			if prog.Flags&elf.PF_W != 0 || (prog.Flags&elf.PF_X == 0 && prog.Flags&elf.PF_R != 0) {
				seg.IsData = true
			}
		}

		segments = append(segments, seg)
	}

	return segments
}

// parseELFSections 解析ELF节
func parseELFSections(f *elf.File) []ELFSection {
	sections := make([]ELFSection, 0)
	sects := f.Sections
	if sects == nil {
		return sections
	}

	for _, sect := range sects {
		sec := ELFSection{
			Name:    sect.Name,
			Addr:    uint64(sect.Addr),
			Offset:  uint64(sect.Offset),
			Size:    uint64(sect.Size),
			Link:    sect.Link,
			Info:    sect.Info,
			Align:   uint64(sect.Addralign),
			EntSize: uint64(sect.Entsize),
		}

		// 解析节类型
		sectionTypeMap := map[elf.SectionType]string{
			elf.SHT_NULL:          "SHT_NULL",
			elf.SHT_PROGBITS:      "SHT_PROGBITS",
			elf.SHT_SYMTAB:        "SHT_SYMTAB",
			elf.SHT_STRTAB:        "SHT_STRTAB",
			elf.SHT_RELA:          "SHT_RELA",
			elf.SHT_HASH:          "SHT_HASH",
			elf.SHT_DYNAMIC:       "SHT_DYNAMIC",
			elf.SHT_NOTE:          "SHT_NOTE",
			elf.SHT_NOBITS:        "SHT_NOBITS",
			elf.SHT_REL:           "SHT_REL",
			elf.SHT_SHLIB:         "SHT_SHLIB",
			elf.SHT_DYNSYM:        "SHT_DYNSYM",
			elf.SHT_INIT_ARRAY:    "SHT_INIT_ARRAY",
			elf.SHT_FINI_ARRAY:    "SHT_FINI_ARRAY",
			elf.SHT_PREINIT_ARRAY: "SHT_PREINIT_ARRAY",
			elf.SHT_GROUP:         "SHT_GROUP",
			elf.SHT_SYMTAB_SHNDX:  "SHT_SYMTAB_SHNDX",
		}
		if name, ok := sectionTypeMap[sect.Type]; ok {
			sec.Type = name
		} else {
			sec.Type = fmt.Sprintf("Unknown (0x%x)", sect.Type)
		}
		if sect.Type == elf.SHT_SYMTAB || sect.Type == elf.SHT_DYNSYM {
			sec.IsSymTab = true
		}
		if sect.Type == elf.SHT_STRTAB {
			sec.IsStrTab = true
		}

		// 解析节标志
		flags := ""
		if sect.Flags&elf.SHF_WRITE != 0 {
			flags += "W"
		}
		if sect.Flags&elf.SHF_ALLOC != 0 {
			flags += "A"
		}
		if sect.Flags&elf.SHF_EXECINSTR != 0 {
			flags += "X"
		}
		if sect.Flags&elf.SHF_MERGE != 0 {
			flags += "M"
		}
		if sect.Flags&elf.SHF_STRINGS != 0 {
			flags += "S"
		}
		if sect.Flags&elf.SHF_INFO_LINK != 0 {
			flags += "I"
		}
		if sect.Flags&elf.SHF_LINK_ORDER != 0 {
			flags += "L"
		}
		if sect.Flags&elf.SHF_OS_NONCONFORMING != 0 {
			flags += "O"
		}
		if sect.Flags&elf.SHF_GROUP != 0 {
			flags += "G"
		}
		if sect.Flags&elf.SHF_TLS != 0 {
			flags += "T"
		}
		if flags == "" {
			flags = "-"
		}
		sec.Flags = flags

		sections = append(sections, sec)
	}

	return sections
}

// openELF 打开ELF文件，只支持文件路径（string）
func openELF(file string) (*elf.File, []byte, error) {
	f, err := elf.Open(file)
	if err != nil {
		return nil, nil, utils.Wrap(err, "open ELF file")
	}

	var rawData []byte
	if fileData, readErr := os.ReadFile(file); readErr == nil && len(fileData) >= 16 {
		rawData = fileData[:16]
	}

	return f, rawData, nil
}

// openELFFromBytes 从字节数组打开ELF文件
func openELFFromBytes(data []byte) (*elf.File, []byte, error) {
	rawData := data
	if len(rawData) > 16 {
		rawData = rawData[:16]
	}
	readerAt := &bytesReaderAt{data: data}
	f, err := elf.NewFile(readerAt)
	if err != nil {
		return nil, nil, utils.Wrap(err, "parse ELF from bytes")
	}
	return f, rawData, nil
}

// convertELFToBytes 将 string 或 []byte 转换为 []byte（ELF专用）
func convertELFToBytes(file interface{}) ([]byte, error) {
	switch v := file.(type) {
	case string:
		data, err := os.ReadFile(v)
		if err != nil {
			return nil, utils.Wrap(err, "read file")
		}
		return data, nil
	case []byte:
		return v, nil
	default:
		// 尝试将 []interface{} 转换为 []byte
		ret := utils.InterfaceToSliceInterface(file)
		if len(ret) == 0 {
			return nil, utils.Errorf("unsupported file type: %T, expected string or []byte", file)
		}
		bytes := make([]byte, 0, len(ret))
		for _, item := range ret {
			switch val := item.(type) {
			case byte:
				// byte 和 uint8 是同一类型
				bytes = append(bytes, val)
			case int:
				if val >= 0 && val <= 255 {
					bytes = append(bytes, byte(val))
				} else {
					return nil, utils.Errorf("invalid byte value: %d", val)
				}
			default:
				if b := utils.InterfaceToBytes(val); len(b) > 0 {
					bytes = append(bytes, b...)
				}
			}
		}
		return bytes, nil
	}
}

// ParseELF 解析ELF文件，返回ELF信息结构
// @param {string|[]byte} file 文件路径或字节数组
// @return {*ELFInfo} ELF文件信息
// @return {error} 错误信息
// Example:
// ```
// // 从文件路径解析
// info, err = elf.ParseELF("/path/to/binary")
// dump(info.Header.Magic)  // "ELF"
// dump(info.Header.Machine)  // "EM_X86_64 (AMD x86-64)"
// dump(info.Header.Entry)  // 入口地址
//
// // 从字节数组解析
// data = file.ReadFile("/path/to/binary")
// info, err = elf.ParseELF(data)
// ```
func ParseELF(file interface{}) (*ELFInfo, error) {
	bytes, err := convertELFToBytes(file)
	if err != nil {
		return nil, err
	}

	f, _, err := openELFFromBytes(bytes)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info := &ELFInfo{
		Header:   parseELFHeader(f),
		Segments: parseELFSegments(f),
		Sections: parseELFSections(f),
	}

	return info, nil
}

// ReadELFHeader 仅读取ELF文件头信息
// @param {string|[]byte} file 文件路径或字节数组
// @return {*ELFHeader} ELF文件头信息
// @return {error} 错误信息
// Example:
// ```
// // 从文件路径读取
// header, err = elf.ReadELFHeader("/path/to/binary")
// // 或从字节数组读取
// data = file.ReadFile("/path/to/binary")
// header, err = elf.ReadELFHeader(data)
// dump(header.Magic)  // "ELF"
// dump(header.Class)  // "64-bit"
// dump(header.Machine)  // "EM_X86_64 (AMD x86-64)"
// dump(header.Entry)  // 入口地址
// ```
func ReadELFHeader(file interface{}) (*ELFHeader, error) {
	info, err := ParseELF(file)
	if err != nil {
		return nil, err
	}
	return info.Header, nil
}

// ReadELFSegments 读取ELF段信息
// @param {string|[]byte} file 文件路径或字节数组
// @return {[]ELFSegment} ELF段信息列表
// @return {error} 错误信息
// Example:
// ```
// // 从文件路径读取
// segments, err = elf.ReadELFSegments("/path/to/binary")
// // 或从字节数组读取
// data = file.ReadFile("/path/to/binary")
// segments, err = elf.ReadELFSegments(data)
//
//	for seg in segments {
//	    if seg.IsCode {
//	        dump(seg.Type, seg.VAddr, seg.FileSz)  // 代码段信息
//	    }
//	    if seg.IsData {
//	        dump(seg.Type, seg.VAddr, seg.FileSz)  // 数据段信息
//	    }
//	}
//
// ```
func ReadELFSegments(file interface{}) ([]ELFSegment, error) {
	info, err := ParseELF(file)
	if err != nil {
		return nil, err
	}
	return info.Segments, nil
}

// ReadELFSections 读取ELF节信息
// @param {string|[]byte} file 文件路径或字节数组
// @return {[]ELFSection} ELF节信息列表
// @return {error} 错误信息
// Example:
// ```
// // 从文件路径读取
// sections, err = elf.ReadELFSections("/path/to/binary")
// // 或从字节数组读取
// data = file.ReadFile("/path/to/binary")
// sections, err = elf.ReadELFSections(data)
//
//	for sect in sections {
//	    if sect.IsSymTab {
//	        dump(sect.Name, sect.Type)  // 符号表信息
//	    }
//	    if sect.IsStrTab {
//	        dump(sect.Name, sect.Type)  // 字符串表信息
//	    }
//	}
//
// ```
func ReadELFSections(file interface{}) ([]ELFSection, error) {
	info, err := ParseELF(file)
	if err != nil {
		return nil, err
	}
	return info.Sections, nil
}

// IsELF 检查文件是否为ELF格式
// @param {string} file 文件路径
// @return {bool} 是否为ELF文件
// Example:
// ```
//
//	if elf.IsELF("/path/to/binary") {
//	    println("This is an ELF file")
//	}
//
// ```
func IsELF(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()

	data := make([]byte, 4)
	_, err = io.ReadFull(f, data)
	if err != nil {
		return false
	}

	return data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F'
}

// GetELFArchitecture 获取ELF文件的架构类型
// @param {string|[]byte} file 文件路径或字节数组
// @return {string} 架构类型字符串
// @return {error} 错误信息
// Example:
// ```
// // 从文件路径获取
// arch, err = elf.GetELFArchitecture("/path/to/binary")
// // 或从字节数组获取
// data = file.ReadFile("/path/to/binary")
// arch, err = elf.GetELFArchitecture(data)
// dump(arch)  // "EM_X86_64 (AMD x86-64)"
// ```
func GetELFArchitecture(file interface{}) (string, error) {
	header, err := ReadELFHeader(file)
	if err != nil {
		return "", err
	}
	return header.Machine, nil
}

// GetELFEntryPoint 获取ELF文件的入口地址
// @param {string|[]byte} file 文件路径或字节数组
// @return {uint64} 入口地址
// @return {error} 错误信息
// Example:
// ```
// // 从文件路径获取
// entry, err = elf.GetELFEntryPoint("/path/to/binary")
// // 或从字节数组获取
// data = file.ReadFile("/path/to/binary")
// entry, err = elf.GetELFEntryPoint(data)
// dump(entry)  // 0x401000
// ```
func GetELFEntryPoint(file interface{}) (uint64, error) {
	header, err := ReadELFHeader(file)
	if err != nil {
		return 0, err
	}
	return header.Entry, nil
}

// GetELFSegment 获取指定索引的ELF段信息
// @param {*ELFInfo} info ELF信息结构
// @param {int} index 段索引
// @return {*ELFSegment} ELF段信息
// @return {error} 错误信息
// Example:
// ```
// info, err = elf.ParseELF("/path/to/binary")
// seg, err = elf.GetELFSegment(info, 0)  // 获取第一个段
// dump(seg.Type, seg.Flags)
// ```
func GetELFSegment(info *ELFInfo, index int) (*ELFSegment, error) {
	if info == nil {
		return nil, utils.Error("ELFInfo is nil")
	}
	if index < 0 || index >= len(info.Segments) {
		return nil, utils.Errorf("segment index out of range: %d (length: %d)", index, len(info.Segments))
	}
	return &info.Segments[index], nil
}

// GetELFSection 获取指定索引的ELF节信息
// @param {*ELFInfo} info ELF信息结构
// @param {int} index 节索引
// @return {*ELFSection} ELF节信息
// @return {error} 错误信息
// Example:
// ```
// info, err = elf.ParseELF("/path/to/binary")
// sect, err = elf.GetELFSection(info, 0)  // 获取第一个节
// dump(sect.Name, sect.Type)
// ```
func GetELFSection(info *ELFInfo, index int) (*ELFSection, error) {
	if info == nil {
		return nil, utils.Error("ELFInfo is nil")
	}
	if index < 0 || index >= len(info.Sections) {
		return nil, utils.Errorf("section index out of range: %d (length: %d)", index, len(info.Sections))
	}
	return &info.Sections[index], nil
}

// formatMagicBytes 格式化ELF魔数字节
func formatMagicBytes(data []byte) string {
	if len(data) < 16 {
		// 如果数据不足16字节，用0填充
		padded := make([]byte, 16)
		copy(padded, data)
		data = padded
	}
	var parts []string
	for i := 0; i < 16; i++ {
		parts = append(parts, fmt.Sprintf("%02x", data[i]))
	}
	return strings.Join(parts, " ")
}

// DisplayELF 以 readelf 风格显示 ELF 文件信息
// @param {string|[]byte} file 文件路径或字节数组
// @return {string} 格式化的 ELF 信息字符串
// @return {error} 错误信息
// Example:
// ```
// // 从文件路径显示
// output, err = elf.DisplayELF("/path/to/binary")
// // 或从字节数组显示
// data = file.ReadFile("/path/to/binary")
// output, err = elf.DisplayELF(data)
// println(output)  // 显示类似 readelf 的输出
// ```
func DisplayELF(file interface{}) (string, error) {
	bytes, err := convertELFToBytes(file)
	if err != nil {
		return "", err
	}

	f, rawData, err := openELFFromBytes(bytes)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info := &ELFInfo{
		Header:   parseELFHeader(f),
		Segments: parseELFSegments(f),
		Sections: parseELFSections(f),
	}

	var buf strings.Builder
	buf.Grow(4096) // 预分配缓冲区大小

	// ELF Header
	buf.WriteString("ELF Header:\n")
	buf.WriteString("  Magic:   ")
	if len(rawData) >= 16 {
		buf.WriteString(formatMagicBytes(rawData))
	} else {
		// 如果无法读取原始数据，使用默认值
		buf.WriteString("7f 45 4c 46 02 01 01 00 00 00 00 00 00 00 00 00")
	}
	buf.WriteString("\n")
	buf.WriteString(fmt.Sprintf("  Class:                             %s\n", info.Header.Class))
	buf.WriteString(fmt.Sprintf("  Data:                              %s\n", info.Header.Data))
	buf.WriteString(fmt.Sprintf("  Version:                           0x%x\n", info.Header.Version))
	buf.WriteString(fmt.Sprintf("  OS/ABI:                            %d\n", info.Header.OSABI))
	buf.WriteString(fmt.Sprintf("  ABI Version:                       %d\n", info.Header.ABIVersion))
	buf.WriteString(fmt.Sprintf("  Type:                              %s\n", info.Header.Type))
	buf.WriteString(fmt.Sprintf("  Machine:                           %s\n", info.Header.Machine))
	buf.WriteString(fmt.Sprintf("  Entry point address:               0x%016x\n", info.Header.Entry))
	buf.WriteString("\n")

	// Program Headers
	if len(info.Segments) > 0 {
		buf.WriteString("Program Headers:\n")
		buf.WriteString("  Type           Offset             VirtAddr           PhysAddr\n")
		buf.WriteString("                 FileSiz            MemSiz              Flags  Align\n")
		for _, seg := range info.Segments {
			buf.WriteString(fmt.Sprintf("  %-14s 0x%016x 0x%016x 0x%016x\n",
				seg.Type, seg.Offset, seg.VAddr, seg.PAddr))
			buf.WriteString(fmt.Sprintf("                 0x%016x 0x%016x  %-5s 0x%x\n",
				seg.FileSz, seg.MemSz, seg.Flags, seg.Align))
		}
		buf.WriteString("\n")
	}

	// Section Headers
	if len(info.Sections) > 0 {
		buf.WriteString("Section Headers:\n")
		buf.WriteString("  [Nr] Name              Type            Address          Off    Size   ES Flg Lk Inf Al\n")
		for i, sect := range info.Sections {
			buf.WriteString(fmt.Sprintf("  [%2d] %-16s %-15s %016x %06x %06x %02x %-3s %2d %3d %2d\n",
				i, sect.Name, sect.Type, sect.Addr, sect.Offset, sect.Size,
				sect.EntSize, sect.Flags, sect.Link, sect.Info, sect.Align))
		}
		buf.WriteString("\n")
		buf.WriteString("Key to Flags:\n")
		buf.WriteString("  W (write), A (alloc), X (execute), M (merge), S (strings), I (info),\n")
		buf.WriteString("  L (link order), O (extra OS processing required), G (group), T (TLS),\n")
		buf.WriteString("  C (compressed), x (unknown), o (OS specific), E (exclude),\n")
		buf.WriteString("  l (large), p (processor specific)\n")
	}

	return buf.String(), nil
}

var ElfExports = map[string]interface{}{
	"ParseELF":           ParseELF,
	"ReadELFHeader":      ReadELFHeader,
	"ReadELFSegments":    ReadELFSegments,
	"ReadELFSections":    ReadELFSections,
	"IsELF":              IsELF,
	"GetELFArchitecture": GetELFArchitecture,
	"GetELFEntryPoint":   GetELFEntryPoint,
	"DisplayELF":         DisplayELF,
	"GetELFSegment":      GetELFSegment,
	"GetELFSection":      GetELFSection,
}
