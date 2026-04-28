package ziputil

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memfile"
	zip "github.com/yaklang/yaklang/common/utils/zipx"
)

// 带密码的 zip 解压入口
// 关键词: zip 解压, 密码 zip 读取

// DeCompressWithOptions 解压 zip 文件，支持 DecompressOption（含密码）。
// 关键词: zip 解压密码, DeCompress
func DeCompressWithOptions(zipFile, dest string, opts ...DecompressOption) error {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return err
	}
	return DeCompressFromRawWithOptions(raw, dest, opts...)
}

// DeCompressFromRawWithOptions 从原始字节解压 zip，支持 DecompressOption（含密码）。
// 关键词: zip 内存解压, DeCompressFromRaw, 密码 zip 解压
func DeCompressFromRawWithOptions(raw []byte, dest string, opts ...DecompressOption) error {
	cfg := newDecompressConfig(opts...)

	absDestFull, err := filepath.Abs(dest)
	if err != nil {
		return utils.Errorf("cannot found dest(%s) abspath: %s", dest, err)
	}
	_ = os.MkdirAll(absDestFull, 0o777)

	size := len(raw)
	mfile := memfile.New(raw)
	reader, err := zip.NewReader(mfile, int64(size))
	if err != nil {
		return utils.Errorf("create zip reader failed: %s", err)
	}

	for _, file := range reader.File {
		filename := filepath.Join(dest, file.Name)
		filenameAbs, err := filepath.Abs(filename)
		if err != nil {
			return utils.Errorf("cannot convert %s as abs path: %s", filename, err)
		}
		if !strings.HasPrefix(filenameAbs, absDestFull) {
			return utils.Errorf("extract file(%s) [abs:%s] is not in [%s]", filename, filenameAbs, absDestFull)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filename, 0o777); err != nil {
				return utils.Errorf("mkdir failed: %s", err)
			}
			continue
		}

		dirName := filepath.Dir(filename)
		if err := os.MkdirAll(dirName, 0o777); err != nil {
			log.Errorf("mkdir [%s] failed: %s", dirName, err)
			return err
		}

		// 加密 zip 条目设置密码
		// 关键词: zip 加密解压, SetPassword
		if file.IsEncrypted() {
			if cfg.Password == "" {
				return utils.Errorf("file %s is encrypted but no password supplied", file.Name)
			}
			file.SetPassword(cfg.Password)
		}

		rc, err := file.Open()
		if err != nil {
			return utils.Errorf("open zip entry %s failed: %s", file.Name, err)
		}

		w, err := os.Create(filename)
		if err != nil {
			rc.Close()
			return err
		}

		if _, err := io.Copy(w, rc); err != nil {
			w.Close()
			rc.Close()
			return utils.Errorf("write zip entry %s failed: %s", file.Name, err)
		}
		w.Close()
		rc.Close()
	}
	return nil
}
