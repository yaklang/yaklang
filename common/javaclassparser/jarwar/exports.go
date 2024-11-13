package jarwar

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
)

// Decompile 反编译一个 jar包或者 class
// 返回值是反编译后的 java 文件路径
// Example:
// ```
// err = Decompile("test.jar", "test/"); die(err)
// err = Decompile("test.war", "test/"); die(err)
// Decompile("a.class", "a.java"); die(err)
// ```
func AutoDecompile(from, to string) error {
	// check from suffix
	if strings.HasSuffix(from, ".jar") || strings.HasSuffix(from, ".war") {
		jar, err := New(from)
		if err != nil {
			return utils.Errorf("create jar/war failed: %v", err)
		}
		return jar.DumpToLocalFileSystem(to)
	} else if strings.HasSuffix(from, ".class") {
		raw, err := os.ReadFile(from)
		if err != nil {
			return utils.Errorf("read class file failed: %v", err)
		}
		stm, err := javaclassparser.Parse(raw)
		if err != nil {
			return utils.Errorf("decompile class failed: %v", err)
		}
		decompiled, err := stm.Dump()
		if err != nil {
			return utils.Errorf("dump class file failed: %v", err)
		}

		// 检查并确保输出文件有 .java 后缀
		if !strings.HasSuffix(to, ".java") {
			to = to + ".java"
		}

		if err := os.WriteFile(to, []byte(decompiled), 0644); err != nil {
			return utils.Errorf("write java file failed: %v", err)
		}
		return nil
	}
	return utils.Errorf("unknown file type: %v", from)
}
