package native

import (
	"debug/elf"
	"os"
	"sort"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func TestDiscoverELFunctionsFromTestBinary(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if file.Machine != elf.EM_X86_64 {
		t.Skip("function boundary discovery is implemented for x86-64")
	}
	image := &core.Image{}
	discoverELFunctions(file, image)
	if len(image.Functions) == 0 {
		t.Fatal("no functions discovered")
	}
	if image.BoundaryStats.Symbol == 0 {
		t.Fatal("no symbol boundaries discovered")
	}
	if image.BoundaryStats.EHFrame == 0 {
		t.Fatal("no eh_frame boundaries discovered")
	}
	sorted := sort.SliceIsSorted(image.Functions, func(i, j int) bool {
		return image.Functions[i].Address < image.Functions[j].Address
	})
	if !sorted {
		t.Fatal("function boundaries are not sorted")
	}
	for _, function := range image.Functions {
		if function.Address == 0 || function.Size == 0 || function.Source == "" {
			t.Fatalf("incomplete boundary: %+v", function)
		}
	}
}
