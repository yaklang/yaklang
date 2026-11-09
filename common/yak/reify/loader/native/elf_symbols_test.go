package native

import (
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func TestELFSymbolsFromSystemBinary(t *testing.T) {
	path := "/bin/true"
	file, err := os.Open(path)
	if err != nil {
		t.Skipf("%s unavailable: %v", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := core.NewByteReader(path, file, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	image, _, _, err := inspectELF(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(image.Symbols) == 0 {
		t.Skipf("%s has no symbols", path)
	}
	found := false
	for _, symbol := range image.Symbols {
		if symbol.Imported && symbol.Address != 0 && symbol.Kind == core.SymbolFunction {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no imported function symbol with an address in %s", path)
	}
}
