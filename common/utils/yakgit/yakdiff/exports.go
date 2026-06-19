package yakdiff

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// DiffZIPFile 比较两个 ZIP 压缩包的内容并返回 git 风格的 diff 文本
// 是对 FileSystemDiff 的高层封装，自动将 ZIP 文件加载为文件系统再比较
// 参数:
//   - zipFile1: 第一个（旧）ZIP 文件路径
//   - zipFile2: 第二个（新）ZIP 文件路径
//   - handler: 可选的差异回调处理器；提供后将逐个变更回调且返回空字符串
//
// 返回值:
//   - diff 文本（未提供 handler 时）
//   - 错误信息
//
// Example:
// ```
// // 比较两个 ZIP 包（示意性示例，需替换为真实路径）
// result, err = diff.DiffZIPFile("/tmp/old.zip", "/tmp/new.zip")
// if err != nil { die(err) }
// println(result)
// ```
func DiffZIPFile(zipFile1, zipFile2 string, handler ...DiffHandler) (string, error) {
	// Check if files exist
	if ok, _ := utils.PathExists(zipFile1); !ok {
		return "", errors.Errorf("zip file %s not existed", zipFile1)
	}
	if ok, _ := utils.PathExists(zipFile2); !ok {
		return "", errors.Errorf("zip file %s not existed", zipFile2)
	}

	// Create ZIP file systems
	fs1, err := filesys.NewZipFSFromLocal(zipFile1)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create zip fs from %s", zipFile1)
	}

	fs2, err := filesys.NewZipFSFromLocal(zipFile2)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create zip fs from %s", zipFile2)
	}

	// Perform filesystem diff
	return FileSystemDiff(fs1, fs2, handler...)
}

var Exports = map[string]any{
	"Diff":               Diff,
	"DiffFromFileSystem": FileSystemDiff,
	"DiffDir": func(i string, j string) (string, error) {
		if ok, _ := utils.PathExists(i); !ok {
			return "", errors.Errorf("path %s not existed", i)
		}
		if ok, _ := utils.PathExists(j); !ok {
			return "", errors.Errorf("path %s not existed", j)
		}
		return FileSystemDiff(filesys.NewRelLocalFs(i), filesys.NewRelLocalFs(j))
	},
	"DiffZIPFile": DiffZIPFile,
}
