package scannode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
)

func TestCreateTempScriptFileFallsBackWhenYakitTempIsNotWritable(t *testing.T) {
	yakitHome := t.TempDir()
	yakitTemp := filepath.Join(yakitHome, "temp")
	if err := os.MkdirAll(yakitTemp, 0o500); err != nil {
		t.Fatalf("mkdir yakit temp: %v", err)
	}
	t.Setenv("YAKIT_HOME", yakitHome)
	consts.ResetYakitHomeOnce()
	t.Cleanup(consts.ResetYakitHomeOnce)

	node := &ScanNode{}
	path, err := node.createTempScriptFile(`println("ok")`)
	if err != nil {
		t.Fatalf("create temp script file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	if strings.HasPrefix(path, yakitTemp+string(os.PathSeparator)) {
		t.Fatalf("expected fallback outside unwritable YAKIT temp, got %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp script file: %v", err)
	}
	if string(raw) != `println("ok")` {
		t.Fatalf("unexpected script content: %q", string(raw))
	}
}
