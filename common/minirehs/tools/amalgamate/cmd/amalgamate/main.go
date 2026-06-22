// Command amalgamate 把 native/mvscan 源拼为单文件发行 (mvscan.c + mvscan.h).
//
// 用法 (仓库根目录):
//
//	go run ./common/minirehs/tools/amalgamate/cmd/amalgamate \
//	    -src common/minirehs/native/mvscan \
//	    -out common/minirehs/native/mvscan/amalgamation
//
// 产物 amalgamation/{mvscan.c,mvscan.h} 是 "宿主丢两个文件即可编" 的零依赖单文件内核.
// 漂移由 mvs_amalgamation_test.go 守护 (产物必须与现场重生一致).
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/minirehs/tools/amalgamate"
)

func main() {
	src := flag.String("src", "common/minirehs/native/mvscan", "native/mvscan source dir")
	out := flag.String("out", "common/minirehs/native/mvscan/amalgamation", "output dir")
	flag.Parse()

	cFile, hFile, err := amalgamate.Build(*src)
	if err != nil {
		log.Fatalf("amalgamate build failed: %v", err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("mkdir out failed: %v", err)
	}
	cPath := filepath.Join(*out, "mvscan.c")
	hPath := filepath.Join(*out, "mvscan.h")
	if err := os.WriteFile(cPath, cFile, 0o644); err != nil {
		log.Fatalf("write %s failed: %v", cPath, err)
	}
	if err := os.WriteFile(hPath, hFile, 0o644); err != nil {
		log.Fatalf("write %s failed: %v", hPath, err)
	}
	log.Printf("amalgamation written: %s (%d bytes), %s (%d bytes)", cPath, len(cFile), hPath, len(hFile))
}
