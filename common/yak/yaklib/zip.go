package yaklib

import (
	"bytes"
	"os"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/ziputil"
	zip "github.com/yaklang/yaklang/common/utils/zipx"
)

var ZipExports = map[string]interface{}{
	"Decompress": ziputil.DeCompress,
	"Compress": func(zipName string, filenames ...string) error {
		return ziputil.CompressByName(filenames, zipName)
	},
	"CompressRaw":      CompressRaw,
	"Recursive":        Recursive,
	"RecursiveFromRaw": RecursiveFromRaw,

	// 带密码的压缩 / 解压便捷接口
	// 关键词: zip 密码压缩, zip 密码解压, AES zip
	"DecompressWithPassword":  DecompressWithPassword,
	"CompressWithPassword":    CompressWithPassword,
	"CompressRawWithPassword": CompressRawWithPassword,

	// Grep 功能
	"GrepRegexp":       ziputil.GrepRegexp,
	"GrepSubString":    ziputil.GrepSubString,
	"GrepRawRegexp":    ziputil.GrepRawRegexp,
	"GrepRawSubString": ziputil.GrepRawSubString,

	// GrepPath 功能 - 搜索文件路径
	"GrepPathRegexp":       ziputil.GrepPathRegexp,
	"GrepPathSubString":    ziputil.GrepPathSubString,
	"GrepPathRawRegexp":    ziputil.GrepPathRawRegexp,
	"GrepPathRawSubString": ziputil.GrepPathRawSubString,

	// Grep 配置选项
	"grepLimit":         ziputil.WithGrepLimit,
	"grepContextLine":   ziputil.WithContext,
	"grepCaseSensitive": ziputil.WithGrepCaseSensitive,
	"grepPassword":      ziputil.WithGrepPassword,

	// 路径过滤选项
	"grepIncludePathSubString": ziputil.WithIncludePathSubString,
	"grepExcludePathSubString": ziputil.WithExcludePathSubString,
	"grepIncludePathRegexp":    ziputil.WithIncludePathRegexp,
	"grepExcludePathRegexp":    ziputil.WithExcludePathRegexp,

	// 文件提取功能
	"ExtractFile":             ziputil.ExtractFile,
	"ExtractFileFromRaw":      ziputil.ExtractFileFromRaw,
	"ExtractFiles":            ziputil.ExtractFiles,
	"ExtractFilesFromRaw":     ziputil.ExtractFilesFromRaw,
	"ExtractByPattern":        ziputil.ExtractByPattern,
	"ExtractByPatternFromRaw": ziputil.ExtractByPatternFromRaw,

	// 带 options 的提取（可携带密码）
	// 关键词: 加密 zip 提取
	"ExtractFileWithOptions":             ziputil.ExtractFileWithOptions,
	"ExtractFileFromRawWithOptions":      ziputil.ExtractFileFromRawWithOptions,
	"ExtractFilesWithOptions":            ziputil.ExtractFilesWithOptions,
	"ExtractFilesFromRawWithOptions":     ziputil.ExtractFilesFromRawWithOptions,
	"ExtractByPatternWithOptions":        ziputil.ExtractByPatternWithOptions,
	"ExtractByPatternFromRawWithOptions": ziputil.ExtractByPatternFromRawWithOptions,
	"extractPassword":                    ziputil.WithExtractPassword,

	// 带 options 的压缩 / 解压（可携带密码与加密方法）
	// 关键词: 加密 zip 压缩, 加密 zip 解压
	"CompressByNameWithOptions":    ziputil.CompressByNameWithOptions,
	"DecompressWithOptions":        ziputil.DeCompressWithOptions,
	"DecompressFromRawWithOptions": ziputil.DeCompressFromRawWithOptions,
	"compressPassword":             ziputil.WithCompressPassword,
	"compressEncryption":           ziputil.WithCompressEncryption,
	"decompressPassword":           ziputil.WithDecompressPassword,

	// 加密方法常量
	// 关键词: zip 加密方法常量
	"StandardEncryption": ziputil.StandardEncryption,
	"AES128":             ziputil.AES128Encryption,
	"AES192":             ziputil.AES192Encryption,
	"AES256":             ziputil.AES256Encryption,

	// GrepResult 处理功能
	"MergeGrepResults": ziputil.MergeGrepResults,
	"RRFRankResults":   RRFRankGrepResults,

	// ZipGrepSearcher - 带缓存的搜索器
	"NewGrepSearcher":        ziputil.NewZipGrepSearcher,
	"NewGrepSearcherFromRaw": ziputil.NewZipGrepSearcherFromRaw,
}

// Recursive 递归遍历一个本地 zip 文件中的所有条目，对每个条目调用回调函数
// 参数:
//   - i: 本地 zip 文件路径
//   - cb: 对每个条目调用的回调函数，参数为 (是否为目录, 条目路径, 文件信息)
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
//
//	zip.Recursive("/tmp/abc.zip", (isDir, pathName, info) => {
//			log.info("isDir: %v, pathName: %v, info: %v", isDir, pathName, info.Name())
//	})~
//
// ```
func Recursive(i any, cb func(isDir bool, pathName string, info os.FileInfo) error) error {
	if result := utils.GetFirstExistedFile(utils.InterfaceToString(i)); result != "" {
		zfs, err := filesys.NewZipFSFromLocal(result)
		if err != nil {
			return utils.Errorf("create zip fs failed: %v", err)
		}
		return filesys.SimpleRecursive(filesys.WithFileSystem(zfs), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
			if cb == nil {
				return utils.Error("zip/callback callback is nil")
			}
			return cb(isDir, pathname, info)
		}))
	}
	return utils.Errorf("file not found: %v", i)
}

// RecursiveFromRaw 递归遍历内存中 zip 原始字节的所有条目，对每个条目调用回调函数
// 参数:
//   - i: zip 文件的原始字节内容
//   - cb: 对每个条目调用的回调函数，参数为 (是否为目录, 条目路径, 文件信息)
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 先在内存中压缩两个文件，再遍历统计条目数量
// raw = zip.CompressRaw({"a.txt": "hello", "b.txt": "world"})~
// count = 0
//
//	zip.RecursiveFromRaw(raw, (isDir, pathName, info) => {
//	    count++
//	    return nil
//	})~
//
// assert count == 2, "RecursiveFromRaw should visit both entries"
// ```
func RecursiveFromRaw(i any, cb func(isDir bool, pathName string, info os.FileInfo) error) error {
	raw := utils.InterfaceToString(i)
	zfs, err := filesys.NewZipFSFromString(raw)
	if err != nil {
		return utils.Errorf("create zip fs failed: %v", err)
	}
	return filesys.SimpleRecursive(filesys.WithFileSystem(zfs), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		if cb == nil {
			return utils.Error("zip in (RecursiveFromRaw) callback is nil")
		}
		return cb(isDir, pathname, info)
	}))
}

// CompressRaw 将一个 map（文件名 -> 内容）在内存中压缩为 zip 字节切片
// 参数:
//   - i: 文件名到内容的映射（值可以是字符串或字节切片）
//
// 返回值:
//   - 压缩后的 zip 字节切片
//   - 错误信息
//
// Example:
// ```
// // 内存压缩后再从字节中提取，验证往返一致
// zipBytes = zip.CompressRaw({"a.txt": "hello world"})~
// content = zip.ExtractFileFromRaw(zipBytes, "a.txt")~
// assert string(content) == "hello world", "CompressRaw then ExtractFileFromRaw should round-trip"
// ```
func CompressRaw(i any) ([]byte, error) {
	if !utils.IsMap(i) {
		return nil, utils.Error("input must be a map")
	}

	var buf bytes.Buffer
	zipFp := zip.NewWriter(&buf)
	count := 0
	for k, v := range utils.InterfaceToGeneralMap(i) {
		log.Infof("start to compress %s size: %v", k, utils.ByteSize(uint64(len(utils.InterfaceToString(v)))))
		kw, err := zipFp.Create(k)
		if err != nil {
			log.Warn(utils.Wrapf(err, "create zip file %s failed", k).Error())
			continue
		}
		count++
		kw.Write([]byte(utils.InterfaceToString(v)))
		zipFp.Flush()
	}
	if count <= 0 {
		return nil, utils.Error("no file compressed")
	}
	zipFp.Flush()
	zipFp.Close()
	return buf.Bytes(), nil
}

// CompressRawWithPassword 与 CompressRaw 类似，但生成带密码（默认 AES-256）加密的 zip 字节。
// 关键词: 内存加密 zip, AES256 zip 创建
// 参数:
//   - i: 文件名到内容的映射
//   - password: 加密密码
//   - encryption: 可选的加密方法（默认 AES256）
//
// 返回值:
//   - 加密后的 zip 字节切片
//   - 错误信息
//
// Example:
// ```
// // 加密压缩后用密码提取，验证往返一致
// zipBytes = zip.CompressRawWithPassword({"s.txt": "secret"}, "123456")~
// content = zip.ExtractFileFromRawWithOptions(zipBytes, "s.txt", zip.extractPassword("123456"))~
// assert string(content) == "secret", "encrypted CompressRaw should round-trip with password"
// ```
func CompressRawWithPassword(i any, password string, encryption ...ziputil.EncryptionMethod) ([]byte, error) {
	if !utils.IsMap(i) {
		return nil, utils.Error("input must be a map")
	}
	method := ziputil.AES256Encryption
	if len(encryption) > 0 {
		method = encryption[0]
	}
	files := make(map[string]interface{})
	for k, v := range utils.InterfaceToGeneralMap(i) {
		files[k] = v
	}
	return ziputil.CompressRawMapWithOptions(files,
		ziputil.WithCompressPassword(password),
		ziputil.WithCompressEncryption(method),
	)
}

// CompressWithPassword 把若干本地文件压缩成带密码（AES-256）的 zip 文件。
// 关键词: 文件加密压缩, AES zip 压缩
// 参数:
//   - zipName: 输出的 zip 文件路径
//   - password: 加密密码
//   - filenames: 一个或多个待压缩的文件路径
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
//
//	zip.CompressWithPassword("/tmp/out.zip", "123456", "/tmp/a.txt", "/tmp/b.txt")~
//
// ```
func CompressWithPassword(zipName, password string, filenames ...string) error {
	return ziputil.CompressByNameWithOptions(filenames, zipName,
		ziputil.WithCompressPassword(password),
		ziputil.WithCompressEncryption(ziputil.AES256Encryption),
	)
}

// DecompressWithPassword 解压带密码的 zip 到目标目录。
// 关键词: 加密 zip 解压, AES zip 解压
// 参数:
//   - zipFile: 待解压的 zip 文件路径
//   - dest: 解压输出的目标目录
//   - password: 解密密码
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
//
//	zip.DecompressWithPassword("/tmp/out.zip", "/tmp/dest", "123456")~
//
// ```
func DecompressWithPassword(zipFile, dest, password string) error {
	return ziputil.DeCompressWithOptions(zipFile, dest,
		ziputil.WithDecompressPassword(password),
	)
}

// RRFRankResults 使用 RRF（Reciprocal Rank Fusion）算法对多组 GrepResult 进行融合排序
// 参数:
//   - results: 待排序的 GrepResult 切片
//
// 返回值:
//   - 重新排序后的 GrepResult 切片
//
// Example:
// ```
// // 在内存 zip 中搜索后对结果进行 RRF 排序
// zipBytes = zip.CompressRaw({"a.txt": "hello\nworld"})~
// results = zip.GrepRawSubString(zipBytes, "world")~
// ranked = zip.RRFRankResults(results)
// assert len(ranked) == len(results), "RRFRankResults should keep all results"
// ```
func RRFRankGrepResults(results []*ziputil.GrepResult) []*ziputil.GrepResult {
	return utils.RRFRankWithDefaultK(results)
}
