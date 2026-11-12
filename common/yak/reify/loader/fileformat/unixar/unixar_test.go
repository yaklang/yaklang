package unixar

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat/elf"
)

// archive builds one ar archive from the given members.
func archive(members ...struct {
	name string
	data []byte
}) []byte {
	result := []byte(archiveMagic)
	for _, member := range members {
		header := fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n",
			member.name+"/", "0", "0", "0", "644", len(member.data))
		result = append(result, header...)
		result = append(result, member.data...)
		if len(member.data)%2 == 1 {
			result = append(result, '\n')
		}
	}
	return result
}

// elfObject builds the smallest ELF relocatable object the loader accepts.
func elfObject() []byte {
	// The ELF header from one 64-bit little-endian relocatable object.
	data := make([]byte, 64)
	copy(data, "\x7fELF")
	data[4] = 2     // 64-bit
	data[5] = 1     // little-endian
	data[6] = 1     // version
	data[16] = 1    // ET_REL
	data[18] = 0x3e // EM_X86_64
	data[20] = 1    // version
	return data
}

func readerFor(t *testing.T, data []byte) *core.ByteReader {
	t.Helper()
	reader, err := core.NewByteReader("test", bytesReaderAt(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func TestMatch(t *testing.T) {
	if !(Loader{}).Match(archive()) {
		t.Fatal("archive magic not accepted")
	}
	if (Loader{}).Match([]byte("not an archive")) {
		t.Fatal("foreign input accepted")
	}
}

func TestMembers(t *testing.T) {
	image := archive(
		struct {
			name string
			data []byte
		}{"first.o", []byte("0123")},
		struct {
			name string
			data []byte
		}{"second.o", []byte("45")},
	)
	members, err := Members(readerFor(t, image))
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}
	if members[0].Name != "first.o" || string(members[0].Data) != "0123" {
		t.Fatalf("first member = %q %q", members[0].Name, members[0].Data)
	}
	if members[1].Name != "second.o" || string(members[1].Data) != "45" {
		t.Fatalf("second member = %q %q", members[1].Name, members[1].Data)
	}
}

func TestOddMemberIsPadded(t *testing.T) {
	image := archive(
		struct {
			name string
			data []byte
		}{"a.o", []byte("123")},
		struct {
			name string
			data []byte
		}{"b.o", []byte("45")},
	)
	members, err := Members(readerFor(t, image))
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}
	if string(members[1].Data) != "45" {
		t.Fatalf("second member = %q", members[1].Data)
	}
}

func TestLoadResolvesMembers(t *testing.T) {
	image := archive(struct {
		name string
		data []byte
	}{"object.o", elfObject()})
	// The archive inherits the architecture of the members it can resolve, so a
	// member the ELF loader rejects leaves the archive without a target.
	SetMemberLoaders([]core.Loader{elf.Loader{}})
	checked, err := (Loader{}).Check(readerFor(t, image))
	if err != nil {
		t.Fatal(err)
	}
	if checked.Sections != 1 {
		t.Fatalf("sections = %d, want 1", checked.Sections)
	}
	if checked.Target.Architecture != core.ArchAMD64 {
		t.Fatalf("architecture = %s", checked.Target.Architecture)
	}
}

// TestCheckRejectsForeignMembers checks that an archive whose members use an
// object format no loader knows is refused. Reporting a zero-width target would
// let the caller treat the archive as loadable and then decode nothing.
func TestCheckRejectsForeignMembers(t *testing.T) {
	image := archive(struct {
		name string
		data []byte
	}{"object.o", []byte("go object v1, not ELF")})
	SetMemberLoaders([]core.Loader{elf.Loader{}})
	if _, err := (Loader{}).Check(readerFor(t, image)); err == nil {
		t.Fatal("archive without a resolvable member accepted")
	}
}
