package yaklib

import (
	"archive/zip"
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/ziputil"
	"os"
)

var ZipExports = map[string]interface{}{
	"Decompress": ziputil.DeCompress,
	"Compress": func(zipName string, filenames ...string) error {
		return ziputil.CompressByName(filenames, zipName)
	},
	"CompressRaw":      CompressRaw,
	"Recursive":        Recursive,
	"RecursiveFromRaw": RecursiveFromRaw,
}

// Recursive Decompress decompresses a zip file to a directory
// Example:
// ```
//
//	zip.Decompress("/tmp/abc.zip", (isDir, pathName, info) => {
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
				return utils.Error("callback is nil")
			}
			return cb(isDir, pathname, info)
		}))
	}
	return utils.Errorf("file not found: %v", i)
}

// RecursiveFromRaw decompresses a zip file to a directory
// Example:
// ```
//
//	raw = file.ReadFile("/tmp/abc.zip")~
//	zip.RecursiveFromRawBytes(raw, (isDir, pathName, info) => {
//			log.info("isDir: %v, pathName: %v, info: %v", isDir, pathName, info.Name())
//	})
//
// ```
func RecursiveFromRaw(i any, cb func(isDir bool, pathName string, info os.FileInfo) error) error {
	raw := utils.InterfaceToString(i)
	zfs, err := filesys.NewZipFSFromString(raw)
	if err != nil {
		return utils.Errorf("create zip fs failed: %v", err)
	}
	return filesys.SimpleRecursive(filesys.WithFileSystem(zfs), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		if cb == nil {
			return utils.Error("callback is nil")
		}
		return cb(isDir, pathname, info)
	}))
}

// CompressRaw compresses a map to a zip file
// Example:
// ```
//
//	zipBytes = zip.CompressRaw({
//		 "a.txt": "hello",
//	     "b.txt": "world",
//	})~
//	zipBytes2, err = zip.CompressRaw({ "a.txt": "hello", "b.txt": file.ReadFile("/tmp/external-file-name.txt")~ })
//
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
