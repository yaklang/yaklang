package minirehs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/minirehs/tools/amalgamate"
)

// TestMVSAmalgamationFresh 是单文件发行件的漂移护栏 (纯 Go, 任意构建都跑): 提交在
// native/mvscan/amalgamation/{mvscan.c,mvscan.h} 的单文件, 必须与从 native/mvscan 源现场
// 重新拼接的结果逐字节一致. 若有人改了 native/mvscan/*.c/.h/.inc 却忘了重生单文件, 本测试
// 立即失败, 并提示重生命令. 这样 amalgamation 的运行期正确性 (已由 minirehs_mvs_amalg 标签
// 下的差分/oracle 矩阵验证) 才能持续等同于源.
//
// 关键词: mvscan, amalgamation, drift guard, single file, regeneration
func TestMVSAmalgamationFresh(t *testing.T) {
	srcDir := filepath.Join("native", "mvscan")
	outDir := filepath.Join(srcDir, "amalgamation")

	gotC, gotH, err := amalgamate.Build(srcDir)
	if err != nil {
		t.Fatalf("amalgamate build: %v", err)
	}

	wantC, err := os.ReadFile(filepath.Join(outDir, "mvscan.c"))
	if err != nil {
		t.Fatalf("read committed mvscan.c: %v", err)
	}
	wantH, err := os.ReadFile(filepath.Join(outDir, "mvscan.h"))
	if err != nil {
		t.Fatalf("read committed mvscan.h: %v", err)
	}

	const hint = "amalgamation drifted from native/mvscan source; regenerate via:\n" +
		"  go run ./common/minirehs/tools/amalgamate/cmd/amalgamate"

	if string(gotC) != string(wantC) {
		t.Fatalf("amalgamation mvscan.c out of date (%d committed vs %d fresh bytes).\n%s",
			len(wantC), len(gotC), hint)
	}
	if string(gotH) != string(wantH) {
		t.Fatalf("amalgamation mvscan.h out of date (%d committed vs %d fresh bytes).\n%s",
			len(wantH), len(gotH), hint)
	}
	t.Logf("amalgamation fresh: mvscan.c=%d bytes, mvscan.h=%d bytes (byte-identical to source regen)",
		len(gotC), len(gotH))
}
