// Package unixar loads the ar archive that Unix linkers use for static
// libraries. The members are ordinary object files, so the archive resolves each
// member with the loader that matches its header.
package unixar

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// archiveMagic is the eight-byte signature that starts every member header.
const archiveMagic = "!<arch>\n"

// memberHeaderSize is the fixed size of one member header.
const memberHeaderSize = 60

// Loader loads ar archives.
type Loader struct{}

var _ core.Loader = Loader{}

func (Loader) Kind() core.Format   { return core.FormatArchive }
func (Loader) Name() string        { return "ar" }
func (Loader) Description() string { return "Unix ar archives; members load with their own format" }
func (Loader) Match(prefix []byte) bool {
	return len(prefix) >= len(archiveMagic) && string(prefix[:len(archiveMagic)]) == archiveMagic
}

// Load builds one artifact from the archive.
func (Loader) Load(ctx context.Context, reader *core.ByteReader) (*core.Artifact, error) {
	return LoadWith(ctx, reader, memberLoaders)
}

// memberLoaders is the member table the catalog registers once.
var memberLoaders []core.Loader

// SetMemberLoaders registers the member table.
func SetMemberLoaders(loaders []core.Loader) { memberLoaders = loaders }

func (Loader) Check(reader *core.ByteReader) (*core.CheckResult, error) {
	members, err := Members(reader)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("ar: archive has no members")
	}
	// The target comes from the first member that resolves, because an archive
	// inherits the architecture of the objects it carries.
	checked := &core.CheckResult{
		Kind:       core.KindNative,
		Format:     core.FormatArchive,
		ObjectType: "ar",
		Sections:   len(members),
	}
	for _, member := range members {
		loaded, err := LoaderFor(memberLoaders, context.Background(), member)
		if err != nil {
			continue
		}
		checked.Target = loaded.Target
		return checked, nil
	}
	// The members use an object format no loader knows, for example the Go
	// object files a compiler emits. Reporting a zero-width target would let the
	// caller treat the archive as loadable.
	return nil, fmt.Errorf("ar: no member uses a supported object format")
}

// Member is one archive entry. Data is the member payload; Name is the resolved
// member name.
type Member struct {
	Name   string
	Offset uint64
	Data   []byte
}

// Members reads the member table of one archive. The symbol table and the long
// name table are metadata, not members: the first is skipped and the second is
// used to resolve names that do not fit the fifteen-byte header field.
func Members(reader *core.ByteReader) ([]Member, error) {
	if reader == nil {
		return nil, fmt.Errorf("ar: nil reader")
	}
	magic, err := reader.Bytes(0, uint64(len(archiveMagic)))
	if err != nil || string(magic) != archiveMagic {
		return nil, fmt.Errorf("ar: not an archive")
	}
	total := uint64(reader.Size())
	longNames := []byte(nil)
	result := make([]Member, 0, 16)
	for offset := uint64(len(archiveMagic)); offset+memberHeaderSize <= total; {
		header, err := reader.Bytes(offset, memberHeaderSize)
		if err != nil && len(header) < memberHeaderSize {
			break
		}
		if len(header) < memberHeaderSize {
			break
		}
		if header[58] != '`' || header[59] != '\n' {
			return nil, fmt.Errorf("ar: bad member header at 0x%x", offset)
		}
		rawName := strings.TrimRight(string(header[0:16]), " ")
		sizeField := strings.TrimRight(string(header[48:58]), " ")
		var size uint64
		if _, err := fmt.Sscanf(sizeField, "%d", &size); err != nil {
			return nil, fmt.Errorf("ar: bad member size %q at 0x%x", sizeField, offset)
		}
		dataOffset := offset + memberHeaderSize
		if dataOffset+size > total {
			size = total - dataOffset
		}
		data, err := reader.Bytes(dataOffset, size)
		if err != nil && len(data) < int(size) {
			return nil, fmt.Errorf("ar: truncated member at 0x%x", offset)
		}
		// BSD ar stores a long member name in the first bytes of the member data
		// as #1/<length>. The object starts after the name, so both the payload
		// and its file offset move forward by that length.
		if length, ok := bsdNameLength(rawName); ok {
			if length > uint64(len(data)) {
				return nil, fmt.Errorf("ar: member name at 0x%x is longer than the member", offset)
			}
			name := strings.TrimRight(string(data[:length]), "\x00")
			data = data[length:]
			dataOffset += length
			if strings.HasPrefix(name, "__.SYMDEF") {
				// The BSD symbol index and string table carry no code.
				offset = offset + memberHeaderSize + size + size%2
				continue
			}
			result = append(result, Member{Name: name, Offset: dataOffset, Data: data})
			offset = offset + memberHeaderSize + size + size%2
			continue
		}
		name, isLongNames := resolveName(rawName, longNames)
		switch {
		case isLongNames:
			// The long-name table itself: keep the bytes for later members.
			longNames = data
		case name == "/" || name == "/SYM64/":
			// The symbol index carries no code.
		default:
			result = append(result, Member{Name: name, Offset: dataOffset, Data: data})
		}
		// Members are aligned to an even offset.
		offset = dataOffset + size + size%2
	}
	return result, nil
}

// bsdNameLength reads a BSD extended member name. The header field holds
// #1/<length>, and the name itself is the first <length> bytes of the payload.
func bsdNameLength(raw string) (uint64, bool) {
	if !strings.HasPrefix(raw, "#1/") {
		return 0, false
	}
	length, err := strconv.ParseUint(strings.TrimSpace(raw[3:]), 10, 64)
	if err != nil {
		return 0, false
	}
	return length, true
}

// resolveName returns the member name. A name of "//" marks the long-name table
// and a slash followed by digits indexes into it.
func resolveName(raw string, longNames []byte) (string, bool) {
	if raw == "//" {
		return "", true
	}
	switch {
	case raw == "/" || raw == "/SYM64/":
		return raw, false
	case strings.HasPrefix(raw, "/") && len(raw) > 1:
		var index int
		if _, err := fmt.Sscanf(raw[1:], "%d", &index); err == nil && index >= 0 && index < len(longNames) {
			rest := longNames[index:]
			end := 0
			for end < len(rest) && rest[end] != '\n' {
				end++
			}
			name := string(rest[:end])
			// GNU ar terminates the name with a slash.
			return strings.TrimSuffix(name, "/"), false
		}
		return raw, false
	}
	// A short name ends with the slash GNU ar appends.
	return strings.TrimSuffix(raw, "/"), false
}

// LoadWith builds one artifact from the archive. The members carry independent
// address spaces: a static library is linked into a program, so the addresses a
// member records are relative to its own section layout. Each member therefore
// gets its own base, spaced so the members never overlap, and every address the
// member reported is shifted by that base. File offsets stay absolute, because
// the artifact reader is the archive file itself.
func LoadWith(ctx context.Context, reader *core.ByteReader, loaders []core.Loader) (*core.Artifact, error) {
	members, err := Members(reader)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("ar: archive has no members")
	}
	image := &core.Image{ObjectType: "ar"}
	hints := &core.BoundaryHints{}
	var target core.Target
	// The base advances past every member: one member's addresses cannot collide
	// with the next one's.
	base := uint64(0)
	const memberStride = 0x100000
	loadedMembers := 0
	for _, member := range members {
		loaded, err := LoaderFor(loaders, ctx, member)
		if err != nil || loaded.Image == nil {
			// A member that no loader matches is data, not a failure.
			continue
		}
		// The member's addresses start at zero, so its own image size decides
		// the space it needs.
		shift := base
		for index := range loaded.Image.Sections {
			section := loaded.Image.Sections[index]
			section.Name = member.Name + ":" + section.Name
			section.VirtualAddress += shift
			// The artifact reader is the archive, so the section offset is the
			// member offset plus the offset inside the member.
			section.FileOffset += member.Offset
			image.Sections = append(image.Sections, section)
		}
		for _, symbol := range loaded.Image.Symbols {
			// Zero is a legitimate address inside a member, so every symbol is
			// shifted; an imported symbol has no address to move.
			if !symbol.Imported {
				symbol.Address += shift
			}
			image.Symbols = append(image.Symbols, symbol)
		}
		if loaded.Hints != nil {
			for _, region := range loaded.Hints.ExecSections {
				hints.ExecSections = append(hints.ExecSections, core.Region{
					Address: region.Address + shift,
					Offset:  region.Offset + member.Offset,
					Size:    region.Size,
				})
			}
			for _, start := range loaded.Hints.FunctionStarts {
				hints.FunctionStarts = append(hints.FunctionStarts, start+shift)
			}
			for _, start := range loaded.Hints.InitTargets {
				hints.InitTargets = append(hints.InitTargets, start+shift)
			}
			if len(loaded.Hints.EHFrame) > 0 && len(hints.EHFrame) == 0 && hints.EHFrameAddr == 0 {
				hints.EHFrame = loaded.Hints.EHFrame
				hints.EHFrameAddr = loaded.Hints.EHFrameAddr + shift
			}
		}
		if target.Bits == 0 && loaded.Target.Bits != 0 {
			// The archive inherits the target of its first resolvable member.
			target = loaded.Target
		}
		base = shift + memberStride
		loadedMembers++
	}
	if loadedMembers == 0 || len(image.Sections) == 0 {
		return nil, fmt.Errorf("ar: no member carries code")
	}
	return &core.Artifact{
		Kind:   core.KindNative,
		Format: core.FormatArchive,
		Image:  image,
		Target: target,
		Hints:  hints,
	}, nil
}

// LoaderFor resolves one archive member with the loader whose Match accepts it.
// The archive imports no other loader package, so the caller supplies the table;
// loader/fileformat/all.go owns it. The member format is decided by its own
// header, not by the archive.
func LoaderFor(loaders []core.Loader, ctx context.Context, member Member) (*core.Artifact, error) {
	if len(member.Data) == 0 {
		return nil, fmt.Errorf("ar: empty member %q", member.Name)
	}
	reader, err := core.NewByteReader(member.Name, bytesReaderAt(member.Data), int64(len(member.Data)))
	if err != nil {
		return nil, err
	}
	for _, loader := range loaders {
		if loader.Kind() == core.FormatArchive {
			continue
		}
		prefix, _ := reader.Bytes(0, core.ProbeSize)
		if !loader.Match(prefix) {
			continue
		}
		loaded, err := loader.Load(ctx, reader)
		if err != nil {
			continue
		}
		if loaded.Target.Architecture == core.ArchUnknown {
			continue
		}
		return loaded, nil
	}
	return nil, fmt.Errorf("ar: no loader matches member %q", member.Name)
}

// bytesReaderAt adapts a byte slice to io.ReaderAt.
func bytesReaderAt(data []byte) readerAt { return readerAt(data) }

type readerAt []byte

func (r readerAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r)) {
		return 0, errEOF
	}
	n := copy(p, r[off:])
	if n < len(p) {
		return n, errEOF
	}
	return n, nil
}

var errEOF = fmt.Errorf("EOF")

var _ sym.Symbol
