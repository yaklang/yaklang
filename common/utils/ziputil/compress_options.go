package ziputil

import (
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	zip "github.com/yaklang/yaklang/common/utils/zipx"
)

// 带密码与加密选项的 zip 压缩入口
// 关键词: zip 压缩, 密码压缩, 加密 zip 创建

// CompressByNameWithOptions 与 CompressByName 行为一致，但支持 CompressOption（含密码、加密方法）。
// 关键词: CompressByName, 密码 zip 写
func CompressByNameWithOptions(files []string, dest string, opts ...CompressOption) error {
	cfg := newCompressConfig(opts...)

	if utils.GetFirstExistedPath(dest) != "" {
		return utils.Errorf("dest[%s] is existed", dest)
	}

	d, err := os.Create(dest)
	if err != nil {
		return utils.Errorf("create zip dest:%v failed: %s", dest, err)
	}
	defer d.Close()

	zipWriter := zip.NewWriter(d)
	defer zipWriter.Close()

	if err := compressFilesIntoWriter(zipWriter, files, cfg); err != nil {
		return err
	}
	return nil
}

// CompressRawMapWithOptions 把 map[string]<bytes-like> 压缩成 zip 字节切片，可选密码加密。
// 关键词: 内存 zip 压缩, 密码 zip 字节, CompressRaw
func CompressRawMapWithOptions(files map[string]interface{}, opts ...CompressOption) ([]byte, error) {
	if files == nil {
		return nil, utils.Error("input files is nil")
	}
	cfg := newCompressConfig(opts...)

	var buf bytes.Buffer
	zipFp := zip.NewWriter(&buf)
	count := 0
	for k, v := range files {
		body := []byte(utils.InterfaceToString(v))
		log.Infof("start to compress %s size: %v", k, utils.ByteSize(uint64(len(body))))

		w, err := newZipEntry(zipFp, k, cfg)
		if err != nil {
			log.Warn(utils.Wrapf(err, "create zip file %s failed", k).Error())
			continue
		}
		count++
		if _, err := w.Write(body); err != nil {
			log.Warn(utils.Wrapf(err, "write zip file %s failed", k).Error())
		}
		zipFp.Flush()
	}
	if count <= 0 {
		_ = zipFp.Close()
		return nil, utils.Error("no file compressed")
	}
	if err := zipFp.Flush(); err != nil {
		log.Warnf("flush zip writer failed: %v", err)
	}
	if err := zipFp.Close(); err != nil {
		return nil, utils.Errorf("close zip writer failed: %v", err)
	}
	return buf.Bytes(), nil
}

// compressFilesIntoWriter 把一组路径写入 zipWriter，按需加密
// 关键词: 压缩多文件, 目录压缩, 加密 zip
func compressFilesIntoWriter(zipWriter *zip.Writer, files []string, cfg *CompressConfig) error {
	compressFile := func(filename string) error {
		log.Infof("zip compress %s", filename)
		fileFp, err := os.Open(filename)
		if err != nil {
			return utils.Errorf("cannot open[%s] as fp: %s", filename, err)
		}
		defer fileFp.Close()

		state, err := fileFp.Stat()
		if err != nil {
			return utils.Errorf("cannot fetch filefp stats: %s", err)
		}

		entryName := path.Join(".", filename)

		if state.IsDir() {
			header, err := zip.FileInfoHeader(state)
			if err != nil {
				return utils.Errorf("file info reader create failed: %s", err)
			}
			header.Name = entryName
			if _, err := zipWriter.CreateHeader(header); err != nil {
				return utils.Errorf("zip create header failed: %s", err)
			}
			return nil
		}

		w, err := newZipEntry(zipWriter, entryName, cfg)
		if err != nil {
			return utils.Errorf("zip create entry %s failed: %s", entryName, err)
		}
		if _, err := io.Copy(w, fileFp); err != nil {
			return utils.Errorf("compress file content failed: %s", err)
		}
		return nil
	}

	for _, filename := range files {
		if strings.Contains(filename, "..") {
			return utils.Errorf("filename[%s] invalid: %s", filename, "cannot contains `..`")
		}
		if utils.IsFile(filename) {
			if err := compressFile(filename); err != nil {
				return err
			}
		}

		if utils.IsDir(filename) {
			absPathName, _ := filepath.Abs(filename)
			if absPathName == "" {
				absPathName = filename
			}
			absPath := filepath.IsAbs(filename)
			var baseDir = filename
			if absPath {
				baseDir = absPathName
			} else {
				baseDir = filename
			}

			infos, err := utils.ReadFilesRecursively(baseDir)
			if err != nil {
				return utils.Errorf("read dirs[%s] failed: %s", baseDir, err)
			}

			for _, info := range infos {
				rPath := info.Path
				if !absPath {
					if strings.HasPrefix(rPath, absPathName) {
						rPath = rPath[len(absPathName):]
						rPath = filepath.Join(filename, rPath)
					}
				}

				log.Infof("found file(dir: %v): %s[%v]", info.IsDir, info.Name, rPath)
				if err := compressFile(rPath); err != nil {
					return utils.Errorf("compress path[%s] in dir[%s] failed: %s", rPath, absPathName, err)
				}
			}
			continue
		}
	}
	return nil
}

// newZipEntry 根据是否设置 password 在 zip writer 上创建明文 / 加密条目
// 关键词: zip Encrypt, AES 写入
func newZipEntry(zw *zip.Writer, name string, cfg *CompressConfig) (io.Writer, error) {
	if cfg != nil && cfg.Password != "" {
		method := cfg.EncryptionMethod
		if method == 0 {
			method = AES256Encryption
		}
		return zw.Encrypt(name, cfg.Password, method)
	}
	return zw.Create(name)
}
