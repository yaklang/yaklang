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

// diffDir 比较两个本地目录的内容并返回 git 风格的 diff 文本（导出名为 diff.DiffDir）
// 递归对比两个目录下的同名文件，输出新增、删除与修改
//
// 参数:
//   - i: 第一个（旧）目录路径
//   - j: 第二个（新）目录路径
//
// 返回值:
//   - git 风格的 diff 文本
//   - 错误信息（目录不存在或比较失败时返回）
//
// Example:
// ```
// base = os.TempDir()
// d1 = file.Join(base, "diff_a"); d2 = file.Join(base, "diff_b")
// file.MkdirAll(d1)~; file.MkdirAll(d2)~
// file.Save(file.Join(d1, "f.txt"), "hello")~
// file.Save(file.Join(d2, "f.txt"), "hello world")~
// result = diff.DiffDir(d1, d2)~
// println(result)
// assert result.Contains("hello world"), "diff should contain the changed content"
// ```
func diffDir(i string, j string) (string, error) {
	if ok, _ := utils.PathExists(i); !ok {
		return "", errors.Errorf("path %s not existed", i)
	}
	if ok, _ := utils.PathExists(j); !ok {
		return "", errors.Errorf("path %s not existed", j)
	}
	return FileSystemDiff(filesys.NewRelLocalFs(i), filesys.NewRelLocalFs(j))
}

var Exports = map[string]any{
	"Diff":               Diff,
	"DiffFromFileSystem": FileSystemDiff,
	"DiffDir":            diffDir,
	"DiffZIPFile":        DiffZIPFile,
}
